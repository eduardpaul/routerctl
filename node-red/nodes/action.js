'use strict';

const { RouterctlError } = require('../lib/client');
const { serverClient, pick, shortTime } = require('../lib/nodeutil');

function parseStatic(text) {
  if (!text || !String(text).trim()) return {};
  const v = JSON.parse(text);
  if (!v || typeof v !== 'object' || Array.isArray(v)) throw new Error('arguments must be a JSON object');
  return v;
}

module.exports = function register(RED) {
  // In/out node: runs a router action through the daemon (LED, WiFi,
  // block a client, speedtest, ...). The daemon's command policy decides
  // what may run; disallowed actions fail with err.code "forbidden".
  function RouterctlActionNode(config) {
    RED.nodes.createNode(this, config);
    const client = serverClient(RED, this, config);
    if (!client) return;
    let fixed;
    try {
      fixed = parseStatic(config.args);
    } catch (err) {
      this.status({ fill: 'red', shape: 'ring', text: 'bad arguments' });
      this.error(`arguments: ${err.message}`);
      return;
    }
    this.on('input', async (msg, send, done) => {
      const device = pick(msg.device, config.device);
      const action = pick(msg.action, config.action);
      try {
        if (!device || !action) throw new Error('needs a router and an action (node settings, msg.device, msg.action)');
        // msg.payload: an object merged over the fixed arguments, or a
        // bare value / "k=v" text (the MQTT payload grammar). Anything else
        // (a timestamp from an inject node) leaves the fixed arguments.
        const p = msg.payload;
        let args = { ...fixed };
        const isEvent = p && typeof p === 'object' && typeof p.type === 'string' && p.id && p.data && typeof p.data === 'object';
        if (isEvent) {
          // A routerctl event wired in (device_new -> block-client): its
          // client MAC is the argument
          args = p.data.mac ? { ...fixed, mac: p.data.mac } : { ...fixed };
        } else if (p && typeof p === 'object' && !Array.isArray(p)) {
          args = { ...fixed, ...p };
        } else if (typeof p === 'string' && p.trim() !== '') {
          if (Object.keys(fixed).length) throw new Error('a text payload cannot be combined with fixed arguments; send an object');
          args = p;
        } else if (typeof p === 'boolean') {
          // true / false drive the action's toggle argument
          const value = p ? 'on' : 'off';
          if (Object.keys(fixed).length === 0) {
            args = value;
          } else {
            const spec = (await client.actions(device)).find((a) => a.name === action);
            if (!spec || !spec.toggle) throw new Error(`${action} is not an on/off action; send an object`);
            args = { ...fixed, [spec.toggle]: value };
          }
        }
        this.status({ fill: 'blue', shape: 'dot', text: `${action}...` });
        const res = await client.action(device, action, args, { requestId: msg._msgid });
        msg.payload = res;
        if (!res.ok) {
          const e = res.error || {};
          this.status({ fill: 'red', shape: 'ring', text: `${action}: ${e.code || 'failed'}` });
          throw new RouterctlError(`${action} on ${device}: ${e.message || 'failed'}`, { code: e.code, body: res });
        }
        this.status({ fill: 'green', shape: 'dot', text: `${action} ok ${shortTime()}` });
        send(msg);
        done();
      } catch (err) {
        if (!(err instanceof RouterctlError)) this.status({ fill: 'red', shape: 'ring', text: 'error' });
        done(err);
      }
    });
  }

  RED.nodes.registerType('routerctl-action', RouterctlActionNode);
};
