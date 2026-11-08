package config

// Example is the annotated example printed by `routerctl config example`.
// It must stay valid: config_test.go parses it.
const Example = `# routerctl configuration
#
# Lookup order: built-in defaults < this file < ROUTERCTL_* env vars < flags.
# Point routerctl at this file with -c/--config or ROUTERCTL_CONFIG.

log:
  level: info          # debug | info | warn | error
  format: text         # text | json

# Device inventory and event state (atomic JSON writes)
state_file: /data/routerctl-state.json

devices:
  # An ASUSWRT router (stock or Merlin), with its AiMesh nodes
  - name: asus
    driver: asus       # auto | asus | zte | fake  (auto: detect from the home page)
    host: 192.168.50.1
    username: admin
    # Prefer password_file (Docker secrets) over an inline password
    password_file: /run/secrets/asus_password
    ssl: false
    port: 0            # 0 = brand default (80 / 443)
    # pin_sha256: "AB:CD:..."   # pin the router's self-signed certificate
    timeout: 30s
    # Poll interval per part of the model; 0 disables a part
    intervals:
      clients: 30s
      wan: 30s
      system: 60s
      traffic: 30s
      nodes: 2m
      firmware: 6h
    options: {}        # brand-specific settings

  # A ZTE fiber ONT in front of the router
  - name: ont
    driver: zte
    host: 192.168.1.1
    username: user
    password_file: /run/secrets/zte_password
    intervals:
      clients: 60s
      traffic: 0       # disabled

presence:
  offline_grace: 3m    # a client must stay offline this long before device_offline
  online_debounce: 0s  # ... and online this long before device_online

inventory:
  baseline: true       # first run: record every client as known, no device_new flood

mqtt:
  enabled: true
  broker: tcp://mqtt:1883      # tcp://, ssl://, ws://, wss://
  username: routerctl
  password_file: /run/secrets/mqtt_password
  client_id: routerctl
  base_topic: routerctl
  discovery: true              # Home Assistant MQTT discovery (retained)
  discovery_prefix: homeassistant
  trackers: known              # device_tracker for: known (approved/labelled) | all | none
  commands: true               # command topics call driver actions
  # max_danger: safe           # stricter than commands.max_danger for MQTT
  block_switches: false        # a "block internet" switch per tracked client
  qos: 1

notify:
  # The "intruder alert"
  - type: ntfy
    url: https://ntfy.sh
    topic: my-home-network
    # token_file: /run/secrets/ntfy_token
    events: [device_new, wan_down, wan_up]
    tags: [house]
  - type: gotify
    url: http://gotify:8080
    token_file: /run/secrets/gotify_token
    events: [firmware_update, node_offline]
    priority: 5
    # Go text/template; fields: .Type .Time .Device .Data and .Summary
    title: "{{ .Device }}: {{ .Type }}"
    message: "{{ .Summary }}"
  - type: webhook              # JSON POST of the event (plus "message")
    url: http://node-red:1880/routerctl
    headers:
      X-Source: routerctl

# One HTTP server: /healthz always, /metrics and /api/v1 when enabled
http:
  listen: "127.0.0.1:9100"     # use ":9100" in Docker (a token is then required)
  # token_file: /run/secrets/routerctl_token   # Authorization: Bearer <token>
  protect_metrics: false       # also require the token on /metrics

# Command policy shared by MQTT, the API and the CLI (each can be stricter)
commands:
  max_danger: disruptive       # safe | disruptive | destructive (reboot & co.)
  # allow: [led, wifi, block-client]
  # deny: [reconnect-wan]

# HTTP API (REST + Server-Sent Events): used by the Node-RED nodes
# (@eduardpaul/node-red-routerctl) and by "routerctl --via"
api:
  enabled: false
  # max_danger: safe
  buffer: 500                  # events kept for stream resumption

metrics:
  enabled: true                # /metrics on the HTTP server
  per_client: true

stdout:
  enabled: false               # events as JSON lines on stdout
  snapshots: false
  # events: [device_new]
`
