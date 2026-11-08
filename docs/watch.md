# Run the daemon

`routerctl watch` polls every configured router, keeps a device inventory
and turns changes into events for MQTT, notifications, the HTTP API,
Prometheus and stdout.

```sh
routerctl watch -c routerctl.yaml
routerctl watch --stdout              # also print events as JSON lines
routerctl watch --api --metrics       # serve the HTTP API and /metrics
```

- Each part (clients, WAN, system, ...) is polled on its own interval.
  Failures back off from 5s up to 5m.
- Ctrl-C or SIGTERM saves the state file and logs out cleanly.
- Logs go to stderr (`log.format: json` for machines).

## Manage the device inventory

Every MAC address ever seen is kept in the state file, with its name,
vendor, IP, first and last seen, a *known* flag and a label. On the first
run every current client is recorded as known, so only devices that
appear later count as new.

```sh
routerctl devices --unknown                          # what joined the network?
routerctl devices approve aa:bb:cc:dd:ee:ff
routerctl devices label aa:bb:cc:dd:ee:ff "Anna's phone"
routerctl devices forget aa:bb:cc:dd:ee:ff           # it counts as new again
```

These work while the daemon runs. With the API on, add
`--via http://127.0.0.1:9100` to edit through the daemon, so the change
is saved at once.

## Events

Every output gets the same JSON:

```json
{"id": "3a7abc31-42", "type": "device_new", "time": "2026-11-08T09:57:54Z",
 "device": "asus", "summary": "New device on asus: visitor (02:00:00:00:20:06), IP 192.168.50.99",
 "data": {"mac": "02:00:00:00:20:06", "name": "visitor", "ip": "192.168.50.99", "known": false}}
```

| Event | Fires when |
|---|---|
| `device_new` | a MAC that is not in the inventory shows up |
| `device_online` / `device_offline` | a client's presence changed and stayed changed (`presence.online_debounce` / `presence.offline_grace`, default 3m) |
| `device_ip_changed` | a known device got a different IP |
| `wan_down` / `wan_up` | the internet connection went down or came back |
| `wan_ip_changed` | the public IP changed |
| `firmware_update` | a firmware update became available (once per version) |
| `node_offline` / `node_online` | a mesh node changed state |
| `action_done` | an action ran (from MQTT, the API or Node-RED) |

## Get alerts

```yaml
notify:
  - type: ntfy                       # ntfy | gotify | webhook
    url: https://ntfy.sh
    topic: my-home
    events: [device_new, wan_down]   # empty = every event
    # devices: [asus]                # only these routers
  - type: gotify
    url: http://gotify:8080
    token_file: /run/secrets/gotify_token
    events: [firmware_update]
  - type: webhook                    # POSTs the event JSON plus title and message
    url: http://n8n:5678/webhook/routerctl
```

To change the text, set `title:` and `message:`. They are Go templates
with `.Type`, `.Time`, `.Device`, `.Data`, `.Title` and `.Summary`, plus
the functions `upper`, `lower`, `json` and `default`:

```yaml
    message: '{{ index .Data "name" }} joined ({{ index .Data "mac" }})'
```

## Export metrics

```yaml
http:
  listen: "127.0.0.1:9100"     # ":9100" in Docker
metrics:
  enabled: true
  per_client: true             # one series per client (online, rssi)
```

- `GET /metrics` serves the Prometheus metrics: `routerctl_up`, CPU, RAM,
  temperatures, WAN, traffic, clients, mesh nodes, firmware, events and
  polls.
- `GET /healthz` returns 503 once a device has failed for 5 minutes. Use
  it as a container health check.
- Set `http.protect_metrics: true` to require the API token on `/metrics`
  too.

## Print events to stdout

```yaml
stdout:
  enabled: true
  events: [device_new]   # empty = every event
  snapshots: false       # also print every poll result
```

Pipe the output into `jq` or any script.
