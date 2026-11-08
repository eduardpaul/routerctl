'use strict';

// The nodes, in a real Node-RED runtime (node-red-node-test-helper),
// against a real routerctl daemon: events arrive, queries and actions
// return contract-shaped payloads, policy refusals are errors, inventory
// edits stick.

const test = require('node:test');
const assert = require('node:assert/strict');
const helper = require('node-red-node-test-helper');
const { startRouterctl, assertConforms, contract } = require('./routerctl');

const serverNode = require('../../nodes/server');
const eventNode = require('../../nodes/event');
const queryNode = require('../../nodes/query');
const actionNode = require('../../nodes/action');
const inventoryNode = require('../../nodes/inventory');

const NODES = [serverNode, eventNode, queryNode, actionNode, inventoryNode];

let rc;

test.before(async () => {
  rc = await startRouterctl();
  helper.init(require.resolve('node-red'));
  await new Promise((r) => helper.startServer(r));
});

test.after(async () => {
  await new Promise((r) => helper.stopServer(r));
  if (rc) await rc.stop();
});

test.afterEach(async () => {
  await helper.unload();
});

function server() {
  return { id: 's1', type: 'routerctl-server', name: 'test', url: rc.url };
}
const creds = () => ({ s1: { token: rc.token } });

function load(flow) {
  return new Promise((resolve, reject) => {
    helper.load(NODES, [server(), ...flow], creds(), (err) => (err ? reject(err) : resolve()));
  });
}

/** Resolves the next message on helper node id (or rejects after ms). */
function next(id, ms = 8000, filter = () => true) {
  const n = helper.getNode(id);
  return new Promise((resolve, reject) => {
    const t = setTimeout(() => reject(new Error(`no message on ${id}`)), ms);
    const on = (msg) => {
      if (!filter(msg)) return;
      clearTimeout(t);
      n.removeListener('input', on);
      resolve(msg);
    };
    n.on('input', on);
  });
}

/** Resolves the next error raised by node id. */
function nextError(id, ms = 8000) {
  const n = helper.getNode(id);
  return new Promise((resolve, reject) => {
    const t = setTimeout(() => reject(new Error(`no error on ${id}`)), ms);
    n.on('call:error', (call) => {
      clearTimeout(t);
      resolve(call);
    });
  });
}

test('contract golden is v1 and knows every event the package lists', () => {
  assert.equal(contract.api, 1);
  for (const t of ['device_new', 'device_online', 'device_offline', 'wan_down', 'action_done']) {
    assert.ok(contract.event_types.includes(t), t);
  }
});

test('event node: device_new and presence from the demo device', async () => {
  await load([
    { id: 'e1', type: 'routerctl-event', server: 's1', types: ['device_new'], wires: [['h1']] },
    { id: 'e2', type: 'routerctl-event', server: 's1', mode: 'presence', macs: '02:00:00:00:10:02', wires: [['h2']] },
    { id: 'h1', type: 'helper' },
    { id: 'h2', type: 'helper' },
  ]);
  const msg = await next('h1', 10000);
  assert.equal(msg.payload.type, 'device_new');
  assert.equal(msg.topic, 'home/device_new');
  assert.equal(msg.payload.data.known, false);
  assert.match(msg.payload.summary, /New device on home/);
  assertConforms(assert, msg.payload, 'Event');
  const pres = await next('h2', 10000);
  assert.ok(['home', 'not_home'].includes(pres.payload), pres.payload);
  assert.equal(pres.topic, 'home/02:00:00:00:10:02');
});

test('query node: clients, device, actions, source and inventory match the contract', async () => {
  await load([
    { id: 'q1', type: 'routerctl-query', server: 's1', what: 'clients', device: 'home', wires: [['h1']] },
    { id: 'h1', type: 'helper' },
  ]);
  const q = helper.getNode('q1');
  const cases = [
    [{}, 'Client', (p) => assert.ok(p.length >= 3)],
    [{ query: 'device' }, 'DeviceDetail', (p) => assert.equal(p.name, 'home')],
    [{ query: 'devices' }, 'Device', (p) => assert.equal(p.length, 1)],
    [{ query: 'nodes' }, 'Node', (p) => assert.equal(p.length, 2)],
    [{ query: 'actions' }, 'Action', (p) => assert.ok(p.find((a) => a.name === 'reboot' && !a.allowed))],
    [{ query: 'sources' }, 'SourceSpec', (p) => assert.ok(p.length > 0)],
    [{ query: 'source', source: 'fake_status' }, 'Source', (p) => assert.equal(p.source, 'fake_status')],
    [{ query: 'inventory' }, 'InventoryEntry', (p) => assert.ok(p.length >= 3)],
    [{ query: 'info' }, 'Info', (p) => assert.equal(p.api, 1)],
  ];
  for (const [over, type, check] of cases) {
    const got = next('h1');
    q.receive({ payload: 1, ...over });
    const msg = await got;
    assertConforms(assert, msg.payload, type);
    check(msg.payload);
  }
  const err = nextError('q1');
  q.receive({ query: 'clients', device: 'nope' });
  const call = await err;
  assert.equal(call.args[0].code, 'unknown_device');
});

test('action node: runs, maps booleans and events, refuses by policy', async () => {
  await load([
    { id: 'a1', type: 'routerctl-action', server: 's1', device: 'home', action: 'led', wires: [['h1']] },
    { id: 'a2', type: 'routerctl-action', server: 's1', device: 'home', action: 'reboot', wires: [['h2']] },
    { id: 'a3', type: 'routerctl-action', server: 's1', device: 'home', action: 'wifi', args: '{"band":"5g1"}', wires: [['h3']] },
    { id: 'a4', type: 'routerctl-action', server: 's1', device: 'home', action: 'block-client', args: '{"state":"on"}', wires: [['h4']] },
    { id: 'h1', type: 'helper' },
    { id: 'h2', type: 'helper' },
    { id: 'h3', type: 'helper' },
    { id: 'h4', type: 'helper' },
  ]);
  let got = next('h1');
  helper.getNode('a1').receive({ payload: false });
  let msg = await got;
  assertConforms(assert, msg.payload, 'Result');
  assert.equal(msg.payload.ok, true);
  assert.deepEqual(msg.payload.args, { state: 'off' });
  assert.equal(msg.payload.id, msg._msgid, 'the message id correlates the request');

  got = next('h1');
  helper.getNode('a1').receive({ payload: 'on' });
  assert.equal((await got).payload.args.state, 'on');

  // A boolean with fixed arguments drives the toggle argument
  got = next('h3');
  helper.getNode('a3').receive({ payload: true });
  msg = await got;
  assert.deepEqual(msg.payload.args, { band: '5g1', state: 'on' });

  // An event wired in: its MAC is the argument
  got = next('h4');
  helper.getNode('a4').receive({ payload: { id: 'x-1', type: 'device_new', device: 'home', data: { mac: '02:00:00:00:10:01' } } });
  msg = await got;
  assert.deepEqual(msg.payload.args, { mac: '02:00:00:00:10:01', state: 'on' });

  // reboot is destructive: the daemon's default policy refuses it
  const err = nextError('a2');
  helper.getNode('a2').receive({ payload: 1 });
  const call = await err;
  assert.equal(call.args[0].code, 'forbidden');
  assert.equal(call.args[1].payload.ok, false);
  assertConforms(assert, call.args[1].payload, 'Result');
});

test('inventory node: device_new -> approve -> known', async () => {
  await load([
    { id: 'e1', type: 'routerctl-event', server: 's1', types: ['device_new'], wires: [['i1']] },
    { id: 'i1', type: 'routerctl-inventory', server: 's1', operation: 'approve', wires: [['h1']] },
    { id: 'i2', type: 'routerctl-inventory', server: 's1', operation: 'label', wires: [['h2']] },
    { id: 'q1', type: 'routerctl-query', server: 's1', what: 'inventory', known: 'false', wires: [['h3']] },
    { id: 'h1', type: 'helper' },
    { id: 'h2', type: 'helper' },
    { id: 'h3', type: 'helper' },
  ]);
  const approved = await next('h1', 15000);
  assertConforms(assert, approved.payload, 'InventoryEntry');
  assert.equal(approved.payload.known, true);
  const mac = approved.payload.mac;

  const labelled = next('h2');
  helper.getNode('i2').receive({ payload: { mac }, label: 'Guest phone' });
  assert.equal((await labelled).payload.label, 'Guest phone');

  const unknown = next('h3');
  helper.getNode('q1').receive({ payload: 1 });
  const list = (await unknown).payload;
  assert.ok(!list.find((e) => e.mac === mac), 'approved device still listed as unknown');
});
