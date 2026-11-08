// Node-RED settings for the end-to-end test: no editor auth, plain
// credentials file (the test writes flows_cred.json itself).
module.exports = {
  flowFile: 'flows.json',
  credentialSecret: false,
  uiPort: 1880,
  httpNodeRoot: '/',
  logging: { console: { level: 'info', metrics: false, audit: false } },
  editorTheme: { tours: false },
};
