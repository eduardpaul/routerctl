'use strict';

const { Selector } = require('../lib/selector');
const { serverClient, bindStreamStatus } = require('../lib/nodeutil');

module.exports = function register(RED) {
  // Source node: routerctl events (device_new, device_online, wan_down,
  // action_done, ...) filtered by type, device, client MAC or unknown-only.
  function RouterctlEventNode(config) {
    RED.nodes.createNode(this, config);
    const client = serverClient(RED, this, config);
    if (!client) return;
    const sel = new Selector({
      types: config.types,
      devices: config.devices,
      macs: config.macs,
      unknownOnly: config.unknownOnly,
    });
    const presence = config.mode === 'presence';
    if (presence) {
      // Presence mode: only the online/offline transitions
      sel.types = ['device_online', 'device_offline'];
    }
    const unbind = bindStreamStatus(this, client);
    const unsubscribe = client.subscribe((e) => {
      if (!sel.match(e)) return;
      const data = e.data || {};
      const msg = presence
        ? {
          topic: `${e.device}/${data.mac}`,
          payload: e.type === 'device_online' ? 'home' : 'not_home',
          event: e,
        }
        : { topic: `${e.device}/${e.type}`, payload: e };
      msg.routerctl = { id: e.id, type: e.type, device: e.device };
      this.send(msg);
    });
    this.on('close', (done) => {
      unsubscribe();
      unbind();
      done();
    });
  }

  RED.nodes.registerType('routerctl-event', RouterctlEventNode);
};
