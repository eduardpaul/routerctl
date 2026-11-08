'use strict';

const { serverClient, pick, shortTime } = require('../lib/nodeutil');

const KINDS = ['info', 'devices', 'device', 'clients', 'nodes', 'actions', 'sources', 'source', 'inventory'];
const NEEDS_DEVICE = new Set(['device', 'clients', 'nodes', 'actions', 'sources', 'source']);

function tri(v) {
  if (v === 'true' || v === true) return true;
  if (v === 'false' || v === false) return false;
  return undefined;
}

module.exports = function register(RED) {
  // In/out node: reads the daemon's view (devices, clients, nodes,
  // actions, native sources, the inventory).
  function RouterctlQueryNode(config) {
    RED.nodes.createNode(this, config);
    const client = serverClient(RED, this, config);
    if (!client) return;
    this.on('input', async (msg, send, done) => {
      const what = pick(msg.query, config.what, 'clients');
      const device = pick(msg.device, config.device);
      try {
        if (!KINDS.includes(what)) throw new Error(`unknown query "${what}" (${KINDS.join(', ')})`);
        if (NEEDS_DEVICE.has(what) && !device) throw new Error(`query "${what}" needs a router (msg.device or the node setting)`);
        let out;
        switch (what) {
          case 'info': out = await client.info(); break;
          case 'devices': out = await client.devices(); break;
          case 'device': out = await client.device(device); break;
          case 'clients':
            out = await client.clients(device, { online: tri(pick(msg.online, config.online)), known: tri(pick(msg.known, config.known)) });
            break;
          case 'nodes': out = await client.nodes(device); break;
          case 'actions': out = await client.actions(device); break;
          case 'sources': out = await client.sources(device); break;
          case 'source': {
            const name = pick(msg.source, config.source);
            if (!name) throw new Error('query "source" needs a source name (msg.source or the node setting)');
            out = await client.source(device, name);
            break;
          }
          default:
            out = await client.inventory({ known: tri(pick(msg.known, config.known)), online: tri(pick(msg.online, config.online)), device: pick(msg.device, config.device) });
        }
        msg.payload = out;
        this.status({ fill: 'green', shape: 'dot', text: `${what} ${Array.isArray(out) ? `(${out.length}) ` : ''}${shortTime()}` });
        send(msg);
        done();
      } catch (err) {
        this.status({ fill: 'red', shape: 'ring', text: err.code || 'error' });
        done(err);
      }
    });
  }

  RED.nodes.registerType('routerctl-query', RouterctlQueryNode);
};
