'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { RouterctlClient, RouterctlError } = require('../../lib/client');

// A stub of the routerctl API: one event per connection, records the
// Last-Event-ID of every stream request.
function stub({ api = 1, token = 't' } = {}) {
  const seen = { lastIds: [], streams: 0, bodies: [] };
  let seq = 0;
  const server = http.createServer((req, res) => {
    if (req.headers.authorization !== `Bearer ${token}`) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: { code: 'auth', message: 'missing or wrong bearer token' } }));
      return;
    }
    if (req.url === '/api/v1/events') {
      seen.streams += 1;
      seen.lastIds.push(req.headers['last-event-id'] || '');
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      seq += 1;
      res.write(`retry: 100\n\nevent: hello\ndata: {"api":${api}}\n\n`);
      res.write(`id: b-${seq}\nevent: device_new\ndata: {"id":"b-${seq}","type":"device_new","device":"home","data":{"mac":"02:00:00:00:00:0${seq}"}}\n\n`);
      // End the stream: the client must reconnect and resume
      setTimeout(() => res.end(), 20);
      return;
    }
    let body = '';
    req.on('data', (c) => { body += c; });
    req.on('end', () => {
      seen.bodies.push({ method: req.method, url: req.url, type: req.headers['content-type'], body });
      if (req.url.startsWith('/api/v1/devices/home/actions/reboot')) {
        res.writeHead(403, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ device: 'home', action: 'reboot', ok: false, error: { code: 'forbidden', message: 'reboot is destructive' } }));
        return;
      }
      if (req.url.startsWith('/api/v1/devices/home/actions/')) {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ device: 'home', action: 'led', ok: true }));
        return;
      }
      if (req.url === '/api/v1/devices/nope') {
        res.writeHead(404, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: { code: 'unknown_device', message: 'unknown device "nope"' } }));
        return;
      }
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify([{ name: 'home' }]));
    });
  });
  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => resolve({ server, seen, url: `http://127.0.0.1:${server.address().port}` }));
  });
}

test('REST calls, errors and action results', async () => {
  const { server, seen, url } = await stub();
  const c = new RouterctlClient({ url, token: 't' });
  try {
    assert.deepEqual(await c.devices(), [{ name: 'home' }]);
    await assert.rejects(c.device('nope'), (e) => e instanceof RouterctlError && e.code === 'unknown_device' && e.status === 404);
    const ok = await c.action('home', 'led', { state: 'off' });
    assert.equal(ok.ok, true);
    // A refused action resolves with the result (ok=false), it does not throw
    const refused = await c.action('home', 'reboot');
    assert.equal(refused.ok, false);
    assert.equal(refused.error.code, 'forbidden');
    await c.action('home', 'led', 'on');
    const text = seen.bodies.find((b) => b.body === 'on');
    assert.equal(text.type, 'text/plain');
    const bad = new RouterctlClient({ url, token: 'wrong' });
    await assert.rejects(bad.info(), (e) => e.code === 'auth' && e.status === 401);
  } finally {
    c.close();
    server.close();
  }
});

test('one shared stream that reconnects and resumes with Last-Event-ID', async () => {
  const { server, seen, url } = await stub();
  const c = new RouterctlClient({ url, token: 't', minBackoff: 20, maxBackoff: 50 });
  const got = [];
  const states = [];
  c.on('status', (s) => states.push(s.state));
  const unsubA = c.subscribe((e) => got.push(`a:${e.id}`));
  const unsubB = c.subscribe((e) => got.push(`b:${e.id}`));
  try {
    await new Promise((resolve, reject) => {
      const t = setTimeout(() => reject(new Error(`events: ${got}`)), 3000);
      const check = setInterval(() => {
        if (got.filter((g) => g.startsWith('a:')).length >= 3) {
          clearTimeout(t);
          clearInterval(check);
          resolve();
        }
      }, 10);
    });
    // Both subscribers share each stream (not one stream each)
    assert.ok(got.includes('a:b-1') && got.includes('b:b-1'));
    assert.deepEqual(seen.lastIds.slice(0, 3), ['', 'b-1', 'b-2']);
    assert.ok(states.includes('connected'));
    unsubA();
    unsubB();
    const streams = seen.streams;
    await new Promise((r) => setTimeout(r, 200));
    assert.ok(seen.streams <= streams + 1, 'stream stopped after the last unsubscribe');
    assert.equal(c.state, 'idle');
  } finally {
    c.close();
    server.close();
  }
});

test('an unsupported API version is reported', async () => {
  const { server, url } = await stub({ api: 2 });
  const c = new RouterctlClient({ url, token: 't', minBackoff: 20, maxBackoff: 5000 });
  try {
    const err = await new Promise((resolve) => {
      c.on('status', (s) => { if (s.state === 'error') resolve(s.error); });
      c.subscribe(() => {});
    });
    assert.match(err, /API v2 is not supported/);
  } finally {
    c.close();
    server.close();
  }
});

test('bad URL', () => {
  assert.throws(() => new RouterctlClient({ url: 'routerctl:9100' }), RouterctlError);
});

test('a stream request that never gets headers is retried', async () => {
  let n = 0;
  const server = http.createServer((req, res) => {
    n += 1;
    if (n === 1) return; // swallow the first request: no headers, ever
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write('event: hello\ndata: {"api":1}\n\nid: b-1\nevent: wan_down\ndata: {"id":"b-1","type":"wan_down"}\n\n');
  });
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const c = new RouterctlClient({ url: `http://127.0.0.1:${server.address().port}`, minBackoff: 10, connectTimeout: 100 });
  try {
    const e = await new Promise((resolve, reject) => {
      const t = setTimeout(() => reject(new Error('no event after a hung connect')), 3000);
      c.subscribe((ev) => { clearTimeout(t); resolve(ev); });
    });
    assert.equal(e.type, 'wan_down');
    assert.ok(n >= 2);
  } finally {
    c.close();
    server.closeAllConnections();
    server.close();
  }
});

test('a silent stream is dropped by the watchdog and resumed', async () => {
  const ids = [];
  const server = http.createServer((req, res) => {
    ids.push(req.headers['last-event-id'] || '');
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    // One event, then silence (no pings): the watchdog must reconnect
    res.write(`event: hello\ndata: {"api":1}\n\nid: b-${ids.length}\nevent: wan_up\ndata: {"id":"b-${ids.length}","type":"wan_up"}\n\n`);
  });
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const c = new RouterctlClient({ url: `http://127.0.0.1:${server.address().port}`, minBackoff: 10, idleTimeout: 150 });
  try {
    await new Promise((resolve, reject) => {
      const t = setTimeout(() => reject(new Error(`streams: ${ids}`)), 3000);
      c.subscribe(() => { if (ids.length >= 2) { clearTimeout(t); resolve(); } });
    });
    assert.deepEqual(ids.slice(0, 2), ['', 'b-1']);
  } finally {
    c.close();
    server.closeAllConnections();
    server.close();
  }
});
