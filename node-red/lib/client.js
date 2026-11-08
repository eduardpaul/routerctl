'use strict';

// Client of the routerctl HTTP API (routerctl/internal/sink/api):
// REST calls and one shared Server-Sent Events stream that reconnects
// with backoff and resumes with Last-Event-ID. Uses the global fetch
// (Node.js 18+); no dependencies.

const { EventEmitter } = require('node:events');
const { SSEParser } = require('./sse');

const API_VERSION = 1;
const PREFIX = '/api/v1';

class RouterctlError extends Error {
  constructor(message, { status = 0, code = 'device', body } = {}) {
    super(message);
    this.name = 'RouterctlError';
    this.status = status;
    this.code = code;
    this.body = body;
  }
}

function enc(s) {
  return encodeURIComponent(String(s));
}

function query(params) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params || {})) {
    if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : '';
}

class RouterctlClient extends EventEmitter {
  /**
   * @param {object} o
   * @param {string} o.url      daemon base URL, e.g. http://routerctl:9100
   * @param {string} [o.token]  bearer token (http.token of the daemon)
   * @param {number} [o.timeout] request timeout in ms (default 30s)
   * @param {number} [o.minBackoff] / [o.maxBackoff] stream reconnect delays
   * @param {number} [o.connectTimeout] stream: ms to wait for the headers
   * @param {number} [o.idleTimeout] stream: ms of silence before reconnecting
   */
  constructor({ url, token, timeout = 30000, minBackoff = 1000, maxBackoff = 30000, connectTimeout = 15000, idleTimeout = 45000, fetch: fetchImpl } = {}) {
    super();
    if (!url || !/^https?:\/\//i.test(url)) throw new RouterctlError(`invalid routerctl URL ${JSON.stringify(url)}`, { code: 'invalid_args' });
    this.base = url.replace(/\/+$/, '');
    this.token = token || '';
    this.timeout = timeout;
    this.minBackoff = minBackoff;
    this.maxBackoff = maxBackoff;
    this.connectTimeout = connectTimeout;
    this.idleTimeout = idleTimeout;
    this.fetch = fetchImpl || globalThis.fetch;
    this.subscribers = 0;
    this.lastEventId = '';
    this.state = 'idle';
    this.closed = false;
    this.abort = null;
    this.setMaxListeners(0);
  }

  headers(extra) {
    const h = { Accept: 'application/json', ...extra };
    if (this.token) h.Authorization = `Bearer ${this.token}`;
    return h;
  }

  /** One API call; resolves the decoded body, rejects with RouterctlError. */
  async request(method, path, body, { timeout = this.timeout, allowError = false, headers } = {}) {
    const ctl = new AbortController();
    const timer = setTimeout(() => ctl.abort(), timeout);
    let resp;
    try {
      const init = { method, headers: this.headers(headers), signal: ctl.signal };
      if (body !== undefined) {
        if (typeof body === 'string') {
          init.body = body;
          init.headers['Content-Type'] = 'text/plain';
        } else {
          init.body = JSON.stringify(body);
          init.headers['Content-Type'] = 'application/json';
        }
      }
      resp = await this.fetch(this.base + PREFIX + path, init);
    } catch (err) {
      clearTimeout(timer);
      const msg = err.name === 'AbortError' ? `timeout after ${timeout} ms` : err.message;
      throw new RouterctlError(`routerctl ${method} ${path}: ${msg}`, { code: err.name === 'AbortError' ? 'timeout' : 'unreachable' });
    }
    let text;
    try {
      text = await resp.text();
    } finally {
      clearTimeout(timer);
    }
    let data = null;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = text;
      }
    }
    if (resp.status >= 300) {
      const e = (data && data.error) || {};
      const err = new RouterctlError(e.message || `routerctl ${method} ${path}: HTTP ${resp.status}`, {
        status: resp.status, code: e.code || 'device', body: data,
      });
      if (allowError && data && typeof data === 'object' && 'ok' in data) return data;
      throw err;
    }
    return data;
  }

  info() { return this.request('GET', '/info'); }
  devices() { return this.request('GET', '/devices'); }
  device(dev) { return this.request('GET', `/devices/${enc(dev)}`); }
  clients(dev, q) { return this.request('GET', `/devices/${enc(dev)}/clients${query(q)}`); }
  nodes(dev) { return this.request('GET', `/devices/${enc(dev)}/nodes`); }
  actions(dev) { return this.request('GET', `/devices/${enc(dev)}/actions`); }
  sources(dev) { return this.request('GET', `/devices/${enc(dev)}/sources`); }
  source(dev, name) { return this.request('GET', `/devices/${enc(dev)}/sources/${enc(name)}`); }
  inventory(q) { return this.request('GET', `/inventory${query(q)}`); }
  entry(mac) { return this.request('GET', `/inventory/${enc(mac)}`); }
  patch(mac, p) { return this.request('PATCH', `/inventory/${enc(mac)}`, p); }
  forget(mac) { return this.request('DELETE', `/inventory/${enc(mac)}`); }

  /**
   * Runs an action. args is an object (JSON) or a string ("on",
   * "k=v k2=v2"). Resolves the commands.Result: a refused or failed
   * action resolves with ok=false and error={code,message} (the caller
   * decides), only transport failures reject. requestId is echoed in
   * the result (and its action_done event).
   */
  action(dev, action, args, { timeout = 4 * 60 * 1000, requestId } = {}) {
    const headers = requestId ? { 'X-Request-Id': String(requestId) } : undefined;
    return this.request('POST', `/devices/${enc(dev)}/actions/${enc(action)}`, args === undefined ? {} : args, { timeout, allowError: true, headers });
  }

  // ---- event stream ----

  /**
   * Adds a listener of the shared event stream; the stream runs while at
   * least one subscriber exists. Returns the unsubscribe function.
   * listener(event) gets every events.Event envelope.
   */
  subscribe(listener) {
    this.on('event', listener);
    this.subscribers += 1;
    if (this.subscribers === 1) this.startStream();
    let done = false;
    return () => {
      if (done) return;
      done = true;
      this.off('event', listener);
      this.subscribers -= 1;
      if (this.subscribers === 0) this.stopStream();
    };
  }

  setState(state, error) {
    this.state = state;
    this.emit('status', { state, error });
  }

  startStream() {
    if (this.running) return;
    this.running = true;
    // A loop sleeping in its backoff from an earlier start sees a newer
    // generation and exits: one stream at a time
    this.gen = (this.gen || 0) + 1;
    this.loop(this.gen).catch(() => {});
  }

  stopStream() {
    this.running = false;
    clearTimeout(this.wakeTimer);
    if (this.wake) this.wake();
    if (this.abort) this.abort.abort();
    this.setState('idle');
  }

  async loop(gen) {
    let backoff = this.minBackoff;
    const alive = () => this.running && !this.closed && gen === this.gen;
    while (alive()) {
      const started = Date.now();
      try {
        this.setState('connecting');
        await this.streamOnce();
        if (!alive()) return;
        this.setState('disconnected', 'stream ended');
      } catch (err) {
        if (!alive()) return;
        this.setState('error', err.message);
        if (err.fatal) {
          // Wrong token / API version: retry slowly
          backoff = this.maxBackoff;
        }
      }
      if (Date.now() - started > 10000) backoff = this.minBackoff;
      await new Promise((r) => {
        this.wake = r;
        this.wakeTimer = setTimeout(r, backoff);
      });
      backoff = Math.min(backoff * 2, this.maxBackoff);
    }
  }

  async streamOnce() {
    this.abort = new AbortController();
    const headers = this.headers({ Accept: 'text/event-stream' });
    if (this.lastEventId) headers['Last-Event-ID'] = this.lastEventId;
    let resp;
    // The headers must come quickly: a request sent on a pooled
    // keep-alive socket whose peer vanished (a daemon container that was
    // restarted) would otherwise wait forever
    const connectTimer = setTimeout(() => this.abort.abort(), this.connectTimeout);
    try {
      resp = await this.fetch(`${this.base}${PREFIX}/events`, { headers, signal: this.abort.signal });
    } catch (err) {
      const why = err.name === 'AbortError' ? `no answer in ${this.connectTimeout} ms` : (err.cause ? err.cause.code || err.cause.message : err.message);
      throw new RouterctlError(`cannot reach routerctl: ${why}`, { code: 'unreachable' });
    } finally {
      clearTimeout(connectTimer);
    }
    if (resp.status !== 200) {
      let body = null;
      try { body = await resp.json(); } catch { /* not JSON */ }
      const e = new RouterctlError((body && body.error && body.error.message) || `event stream: HTTP ${resp.status}`, { status: resp.status, code: (body && body.error && body.error.code) || 'device' });
      e.fatal = resp.status === 401 || resp.status === 404;
      throw e;
    }
    // Watchdog: the server pings every 15s; idleTimeout (45s) of
    // silence means the connection is dead
    let watchdog;
    const abort = this.abort;
    const arm = () => {
      clearTimeout(watchdog);
      watchdog = setTimeout(() => abort.abort(), this.idleTimeout);
    };
    arm();
    let versionError = null;
    const parser = new SSEParser({
      onEvent: (f) => {
        let data;
        try { data = JSON.parse(f.data); } catch { return; }
        if (f.event === 'hello') {
          if (data.api !== API_VERSION) {
            versionError = new RouterctlError(`routerctl API v${data.api} is not supported (this package speaks v${API_VERSION})`, { code: 'unsupported' });
            versionError.fatal = true;
            this.abort.abort();
            return;
          }
          this.setState('connected');
          this.emit('hello', data);
          return;
        }
        if (f.id) this.lastEventId = f.id;
        if (f.event === 'state') this.emit('state', data);
        else this.emit('event', data);
      },
    });
    const decoder = new TextDecoder();
    try {
      for await (const chunk of resp.body) {
        arm();
        parser.feed(decoder.decode(chunk, { stream: true }));
      }
    } catch (err) {
      if (versionError) throw versionError;
      if (!this.running) return;
      throw new RouterctlError(`event stream: ${err.message}`, { code: 'unreachable' });
    } finally {
      clearTimeout(watchdog);
    }
    if (versionError) throw versionError;
  }

  close() {
    this.closed = true;
    this.running = false;
    clearTimeout(this.wakeTimer);
    if (this.wake) this.wake();
    if (this.abort) this.abort.abort();
    this.removeAllListeners('event');
    this.subscribers = 0;
  }
}

module.exports = { RouterctlClient, RouterctlError, API_VERSION };
