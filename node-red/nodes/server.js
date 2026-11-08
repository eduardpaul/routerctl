'use strict';

const { RouterctlClient } = require('../lib/client');

module.exports = function register(RED) {
  // Config node: one routerctl daemon (its HTTP API). Holds the one
  // client, so every node of a server shares a single event stream.
  function RouterctlServerNode(config) {
    RED.nodes.createNode(this, config);
    this.name = config.name;
    this.url = config.url;
    const token = (this.credentials && this.credentials.token) || '';
    try {
      this.client = new RouterctlClient({ url: config.url, token });
    } catch (err) {
      this.error(err.message);
      this.client = null;
    }
    this.on('close', (done) => {
      if (this.client) this.client.close();
      done();
    });
  }

  RED.nodes.registerType('routerctl-server', RouterctlServerNode, {
    credentials: { token: { type: 'password' } },
  });

  // Editor helpers: device / action / source lists for the dropdowns.
  // They proxy through the runtime so the token never reaches the
  // browser; they need a deployed server node.
  const perm = RED.auth.needsPermission('routerctl-server.read');
  const withClient = (fn) => async (req, res) => {
    const node = RED.nodes.getNode(req.params.id);
    if (!node || !node.client) {
      res.status(404).json({ error: { code: 'not_found', message: 'deploy the routerctl server node first' } });
      return;
    }
    try {
      res.json(await fn(node.client, req));
    } catch (err) {
      res.status(err.status && err.status < 600 ? err.status : 502).json({ error: { code: err.code || 'device', message: err.message } });
    }
  };
  RED.httpAdmin.get('/routerctl/:id/devices', perm, withClient((c) => c.devices()));
  RED.httpAdmin.get('/routerctl/:id/actions', perm, withClient((c, req) => c.actions(String(req.query.device || ''))));
  RED.httpAdmin.get('/routerctl/:id/sources', perm, withClient((c, req) => c.sources(String(req.query.device || ''))));
  RED.httpAdmin.get('/routerctl/:id/info', perm, withClient((c) => c.info()));
};
