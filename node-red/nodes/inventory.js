'use strict';

const { serverClient, pick, macOf, shortTime } = require('../lib/nodeutil');

const OPS = ['approve', 'unapprove', 'label', 'forget', 'get'];

module.exports = function register(RED) {
  // In/out node: edits the device inventory of the daemon (approve a new
  // device, label it, forget it). The daemon saves the change at once.
  function RouterctlInventoryNode(config) {
    RED.nodes.createNode(this, config);
    const client = serverClient(RED, this, config);
    if (!client) return;
    this.on('input', async (msg, send, done) => {
      const op = pick(msg.operation, config.operation, 'approve');
      const mac = macOf(msg, config.mac);
      try {
        if (!OPS.includes(op)) throw new Error(`unknown operation "${op}" (${OPS.join(', ')})`);
        if (!mac) throw new Error('needs a MAC address (msg.mac, msg.payload.mac, an event payload or the node setting)');
        let out;
        switch (op) {
          case 'approve': out = await client.patch(mac, { known: true }); break;
          case 'unapprove': out = await client.patch(mac, { known: false }); break;
          case 'label': {
            const label = pick(msg.label, config.label);
            if (label === undefined) throw new Error('label needs msg.label or the node setting');
            out = await client.patch(mac, { label: String(label) });
            break;
          }
          case 'forget':
            await client.forget(mac);
            out = { mac, forgotten: true };
            break;
          default: out = await client.entry(mac);
        }
        msg.payload = out;
        this.status({ fill: 'green', shape: 'dot', text: `${op} ${mac} ${shortTime()}` });
        send(msg);
        done();
      } catch (err) {
        this.status({ fill: 'red', shape: 'ring', text: `${op}: ${err.code || 'error'}` });
        done(err);
      }
    });
  }

  RED.nodes.registerType('routerctl-inventory', RouterctlInventoryNode);
};
