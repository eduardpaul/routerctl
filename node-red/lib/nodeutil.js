'use strict';

// Helpers shared by the nodes.

const { normalizeMAC } = require('./selector');

/** The client of a node's server config node, or null (with a status). */
function serverClient(RED, node, config) {
  const server = RED.nodes.getNode(config.server);
  if (!server || !server.client) {
    node.status({ fill: 'red', shape: 'ring', text: 'no server' });
    return null;
  }
  return server.client;
}

/** Mirrors the event stream state on the node status. */
function bindStreamStatus(node, client) {
  const show = ({ state, error }) => {
    switch (state) {
      case 'connected':
        node.status({ fill: 'green', shape: 'dot', text: 'connected' });
        break;
      case 'connecting':
        node.status({ fill: 'yellow', shape: 'ring', text: 'connecting' });
        break;
      case 'idle':
        node.status({});
        break;
      default:
        node.status({ fill: 'red', shape: 'ring', text: error ? String(error).slice(0, 60) : state });
    }
  };
  client.on('status', show);
  show({ state: client.state === 'idle' ? 'connecting' : client.state });
  return () => client.off('status', show);
}

/** First non-empty value. */
function pick(...vals) {
  for (const v of vals) {
    if (v !== undefined && v !== null && v !== '') return v;
  }
  return undefined;
}

/**
 * The MAC an inventory / presence message is about: msg.mac, a string
 * payload, payload.mac or the payload of an event (payload.data.mac).
 */
function macOf(msg, fallback) {
  const p = msg.payload;
  const cand = pick(
    msg.mac,
    typeof p === 'string' ? p : undefined,
    p && typeof p === 'object' ? p.mac : undefined,
    p && typeof p === 'object' && p.data ? p.data.mac : undefined,
    fallback,
  );
  return cand === undefined ? '' : normalizeMAC(String(cand)) || String(cand);
}

/** A short status text for a result. */
function shortTime() {
  return new Date().toLocaleTimeString();
}

module.exports = { serverClient, bindStreamStatus, pick, macOf, shortTime };
