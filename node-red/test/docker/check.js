'use strict';

// Assertions of the Docker end-to-end test, run against a Node-RED
// container that runs flows.json:
//
//   node check.js <node-red url> main             palette, editor endpoints, flows
//   node check.js <node-red url> boots > f.json   the daemon runs seen so far
//   node check.js <node-red url> restart f.json   events resume after routerctl restarts
//
// (boots runs before the restart: a snapshot taken after it may already
// hold events of the new run)
//
// The flow stores what the routerctl nodes emit in global context and
// serves it on GET /e2e.

const assert = require('node:assert/strict');

const [, , base, phase = 'main', bootsFile] = process.argv;

async function get(path, accept = 'application/json') {
  const r = await fetch(base + path, { headers: { Accept: accept } });
  const text = await r.text();
  return { status: r.status, text, json: () => JSON.parse(text) };
}

async function until(what, fn, ms = 90000) {
  const deadline = Date.now() + ms;
  let last;
  for (;;) {
    try {
      const v = await fn();
      if (v) return v;
    } catch (err) {
      last = err;
    }
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}${last ? `: ${last.message}` : ''}`);
    await new Promise((r) => setTimeout(r, 1000));
  }
}

const readout = async () => (await get('/e2e')).json();
const bootOf = (e) => String(e.id).split('-')[0];

function ok(msg) {
  console.log(`  ok  ${msg}`);
}

async function main() {
  await until('Node-RED', async () => (await get('/e2e')).status === 200);
  ok('Node-RED is up and runs the flow');

  // The palette: the package loaded, five node types, no load errors
  const nodes = (await get('/nodes')).json();
  const mod = nodes.filter((n) => n.module === '@eduardpaul/node-red-routerctl');
  const types = mod.flatMap((n) => n.types).sort();
  assert.deepEqual(types, ['routerctl-action', 'routerctl-event', 'routerctl-inventory', 'routerctl-query', 'routerctl-server']);
  for (const n of mod) assert.ok(n.enabled !== false && !n.err, `${n.name}: ${n.err}`);
  ok(`palette: ${types.join(', ')} (version ${mod[0].version})`);

  const html = (await get('/nodes', 'text/html')).text;
  for (const t of types) assert.ok(html.includes(`data-template-name="${t}"`), `editor template of ${t}`);
  ok('editor templates and help are served');

  // Editor helpers proxy to the daemon (the token stays in the runtime)
  const devs = (await get('/routerctl/e2e_server/devices')).json();
  assert.equal(devs[0].name, 'home');
  const acts = (await get('/routerctl/e2e_server/actions?device=home')).json();
  assert.ok(acts.find((a) => a.name === 'led' && a.allowed));
  assert.ok(acts.find((a) => a.name === 'reboot' && !a.allowed));
  ok(`editor dropdowns: devices [${devs.map((d) => d.name)}], ${acts.length} actions (reboot refused by policy)`);

  const r = await until('the demo events, presence, approval, queries and actions', async () => {
    const o = await readout();
    const seen = new Set((o.events || []).map((e) => e.type));
    const need = ['device_new', 'device_offline', 'device_online', 'wan_down', 'wan_up', 'action_done'];
    const pres = new Set(o.presence || []);
    const forbidden = (o.errors || []).find((e) => e.source === 'a_reboot' && e.code === 'forbidden');
    const missing = [
      ...need.filter((t) => !seen.has(t)),
      ...(pres.has('home') && pres.has('not_home') ? [] : ['presence home+not_home']),
      ...((o.approved || []).length ? [] : ['approved']),
      ...((o.clients || []).length >= 3 ? [] : ['clients']),
      ...(o.device && o.device.name === 'home' ? [] : ['device']),
      ...(o.action && o.action.ok ? [] : ['action']),
      ...(forbidden ? [] : ['forbidden reboot']),
    ];
    if (missing.length) throw new Error(`missing: ${missing.join(', ')}`);
    return o;
  });
  const types2 = [...new Set(r.events.map((e) => e.type))].sort();
  ok(`events: ${types2.join(', ')} (${r.events.length} total)`);
  const nd = r.events.find((e) => e.type === 'device_new');
  assert.match(nd.summary, /New device on home/);
  assert.ok(nd.id && nd.time && nd.data.mac);
  ok(`device_new: "${nd.summary}"`);
  ok(`presence of 02:00:00:00:10:02: ${[...new Set(r.presence)].join(' / ')}`);
  assert.equal(r.approved[0].known, true);
  ok(`intruder flow: device_new -> inventory approve -> ${r.approved[0].mac} known=${r.approved[0].known}`);
  assert.ok(r.clients.every((c) => typeof c.present === 'boolean' && c.mac));
  ok(`query: ${r.clients.length} clients, router ${r.device.identity.model} WAN ${r.device.state.wan_ip}`);
  assert.deepEqual(r.action.args, { state: 'off' });
  ok(`action: led -> ok, args ${JSON.stringify(r.action.args)}`);
  const done = r.events.find((e) => e.type === 'action_done' && e.data.action === 'led');
  assert.equal(done.data.source, 'api');
  assert.ok(done.data.request_id, 'the Node-RED message id is the request id');
  ok(`action_done event from source=api, request_id=${done.data.request_id}`);
  ok('reboot refused by the daemon policy (Catch node got code=forbidden)');
}

async function boots() {
  const o = await readout();
  process.stdout.write(JSON.stringify([...new Set((o.events || []).map(bootOf))]));
}

async function restart() {
  const boots = new Set(JSON.parse(require('node:fs').readFileSync(bootsFile, 'utf8')));
  let r;
  try {
    r = await until('events from the restarted daemon', async () => {
      const o = await readout();
      const fresh = o.events.filter((e) => !boots.has(bootOf(e)));
      return fresh.length ? fresh : null;
    }, 60000);
  } catch (err) {
    const o = await readout();
    console.error(`  event node status history:\n    ${(o.status || []).join('\n    ')}`);
    throw err;
  }
  ok(`after a routerctl restart the stream reconnected: ${r.length} new events (boot ${bootOf(r[0])}, was ${[...boots].join(',')})`);
}

({ main, boots, restart }[phase] || main)().catch((err) => {
  console.error(`  FAIL ${err.message}`);
  process.exit(1);
});
