'use strict';

// Test fixture: a real `routerctl watch --api` with the fake driver in
// demo mode (its state changes on a fixed poll schedule: a client goes
// offline and back, a "visitor" joins, the WAN drops and comes back).
// The binary is $ROUTERCTL_BIN, else built from the repository root.

const { spawn, execFileSync } = require('node:child_process');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');

const GO = path.resolve(__dirname, '../../..');

function freePort() {
  return new Promise((resolve, reject) => {
    const s = net.createServer();
    s.listen(0, '127.0.0.1', () => {
      const { port } = s.address();
      s.close(() => resolve(port));
    });
    s.on('error', reject);
  });
}

function binary(dir) {
  if (process.env.ROUTERCTL_BIN) return process.env.ROUTERCTL_BIN;
  const out = path.join(dir, 'routerctl');
  execFileSync('go', ['build', '-o', out, './routerctl/cmd/routerctl'], { cwd: GO, stdio: 'inherit' });
  return out;
}

async function startRouterctl({ token = 'contract-token', interval = '250ms' } = {}) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'routerctl-contract-'));
  const bin = binary(dir);
  const port = await freePort();
  const parts = ['clients', 'wan', 'system', 'traffic', 'nodes', 'firmware'].map((p) => `${p}: ${interval}`).join(', ');
  const cfg = path.join(dir, 'routerctl.yaml');
  fs.writeFileSync(cfg, [
    `state_file: ${path.join(dir, 'state.json')}`,
    'devices:',
    '  - name: home',
    '    driver: fake',
    '    host: demo',
    `    intervals: {${parts}}`,
    'presence: {offline_grace: 1ms}',
    'api: {enabled: true}',
    `http: {listen: "127.0.0.1:${port}"}`,
    'log: {level: warn}',
    '',
  ].join('\n'));
  const env = { ...process.env, ROUTERCTL_HTTP_TOKEN: token };
  const proc = spawn(bin, ['watch', '-c', cfg], { env, stdio: ['ignore', 'inherit', 'inherit'] });
  const url = `http://127.0.0.1:${port}`;
  const deadline = Date.now() + 15000;
  for (;;) {
    try {
      const r = await fetch(`${url}/api/v1/info`, { headers: { Authorization: `Bearer ${token}` } });
      if (r.status === 200) break;
    } catch { /* not up yet */ }
    if (Date.now() > deadline) {
      proc.kill();
      throw new Error('routerctl did not start');
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  return {
    url,
    token,
    stop: () => new Promise((resolve) => {
      proc.on('exit', () => {
        fs.rmSync(dir, { recursive: true, force: true });
        resolve();
      });
      proc.kill('SIGTERM');
    }),
  };
}

// ---- the contract (routerctl/internal/sink/api/testdata) ----

const contract = JSON.parse(fs.readFileSync(path.join(GO, 'routerctl/internal/sink/api/testdata/contract.golden.json'), 'utf8'));

/**
 * Checks value against a schema of contract.golden.json: no unknown keys,
 * non-omitempty keys present, and the JSON kinds of leaves. Returns the
 * list of problems (empty when it conforms).
 */
function conform(value, schema, where = '$') {
  const errs = [];
  if (value === null || value === undefined) return errs; // pointers, nil lists
  if (typeof schema === 'string') {
    const kind = {
      string: 'string', bool: 'boolean', int: 'number', int64: 'number', uint64: 'number', float64: 'number',
      'time.Time': 'string', 'routerkit.Danger': 'string', 'routerkit.Support': 'string', 'time.Duration': 'number',
    }[schema];
    if (kind && typeof value !== kind) errs.push(`${where}: ${JSON.stringify(value)} is not ${schema}`);
    return errs;
  }
  if (schema.ptr) return conform(value, schema.ptr, where);
  if (schema.list) {
    if (!Array.isArray(value)) return [`${where}: not a list`];
    value.forEach((v, i) => errs.push(...conform(v, schema.list, `${where}[${i}]`)));
    return errs;
  }
  if (schema.map) {
    if (typeof value !== 'object') return [`${where}: not an object`];
    for (const [k, v] of Object.entries(value)) errs.push(...conform(v, schema.of, `${where}.${k}`));
    return errs;
  }
  if (typeof value !== 'object' || Array.isArray(value)) return [`${where}: not an object`];
  for (const k of Object.keys(value)) {
    if (!(k in schema)) errs.push(`${where}: unknown field "${k}"`);
  }
  for (const [k, f] of Object.entries(schema)) {
    if (!f.omitempty && !(k in value)) errs.push(`${where}: missing field "${k}"`);
    if (k in value) errs.push(...conform(value[k], f.type, `${where}.${k}`));
  }
  return errs;
}

function assertConforms(assert, value, typeName) {
  const schema = contract.types[typeName];
  assert.ok(schema, `no contract type ${typeName}`);
  const errs = Array.isArray(value) ? value.flatMap((v, i) => conform(v, schema, `${typeName}[${i}]`)) : conform(value, schema, typeName);
  assert.deepEqual(errs, [], `${typeName} does not match the contract`);
}

module.exports = { startRouterctl, contract, conform, assertConforms };
