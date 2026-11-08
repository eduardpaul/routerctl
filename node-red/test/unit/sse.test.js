'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { SSEParser } = require('../../lib/sse');
const { Selector, normalizeMAC } = require('../../lib/selector');

function parse(chunks) {
  const out = [];
  const retries = [];
  const p = new SSEParser({ onEvent: (e) => out.push(e), onRetry: (r) => retries.push(r) });
  for (const c of chunks) p.feed(c);
  return { out, retries, p };
}

test('frames, ids, comments and retry', () => {
  const { out, retries } = parse([
    'retry: 3000\n\n',
    'event: hello\ndata: {"api":1}\n\n',
    ': ping 123\n\n',
    'id: b-1\nevent: device_new\ndata: {"a":1}\n\n',
  ]);
  assert.deepEqual(retries, [3000]);
  assert.deepEqual(out, [
    { event: 'hello', id: '', data: '{"api":1}' },
    { event: 'device_new', id: 'b-1', data: '{"a":1}' },
  ]);
});

test('chunk boundaries anywhere, CRLF and multi-line data', () => {
  const text = 'id: x-2\r\nevent: wan_down\r\ndata: line1\r\ndata: line2\r\n\r\ndata:nospace\n\n';
  for (let cut = 1; cut < text.length; cut += 1) {
    const { out } = parse([text.slice(0, cut), text.slice(cut)]);
    assert.equal(out.length, 2, `cut at ${cut}`);
    assert.deepEqual(out[0], { event: 'wan_down', id: 'x-2', data: 'line1\nline2' });
    // The last id carries over; the default event type is "message"
    assert.deepEqual(out[1], { event: 'message', id: 'x-2', data: 'nospace' });
  }
});

test('event without data is not dispatched', () => {
  const { out } = parse(['event: x\n\n']);
  assert.equal(out.length, 0);
});

test('selector mirrors events.Selector', () => {
  const newDev = { type: 'device_new', device: 'asus', data: { mac: 'aa:bb:cc:00:00:01', known: false } };
  const known = { type: 'device_online', device: 'asus', data: { mac: 'aa:bb:cc:00:00:02', known: true } };
  const wan = { type: 'wan_down', device: 'zte', data: {} };
  const cases = [
    [{}, [true, true, true]],
    [{ devices: 'zte' }, [false, false, true]],
    [{ macs: ['AA-BB-CC-00-00-02'] }, [false, true, false]],
    [{ unknownOnly: true }, [true, false, false]],
    [{ types: 'device_new, wan_down', devices: ['asus'] }, [true, false, false]],
  ];
  for (const [opts, want] of cases) {
    const s = new Selector(opts);
    assert.deepEqual([newDev, known, wan].map((e) => s.match(e)), want, JSON.stringify(opts));
  }
  assert.equal(normalizeMAC('AABBCCDDEEFF'), 'aa:bb:cc:dd:ee:ff');
  assert.equal(normalizeMAC('aa:bb'), '');
  assert.equal(normalizeMAC('zz:bb:cc:dd:ee:ff'), '');
});
