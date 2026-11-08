# @eduardpaul/node-red-routerctl

Node-RED nodes for [routerctl](https://github.com/eduardpaul/routerctl#readme):
react to router events and run router actions from your flows.

- An unknown device joins → notify, then approve it from the notification.
- A phone comes home or leaves → presence automations.
- The internet goes down → reconnect it, or alert.
- A schedule → pause a child's internet (block a client).

Node-RED never logs in to the router. The routerctl daemon does, and the
nodes talk to its local HTTP API.

## Install

1. Run routerctl with the API on (Docker example):

   ```yaml
   # routerctl.yaml
   devices:
     - name: router
       host: 192.168.50.1
       username: admin
       password_file: /run/secrets/router_password
   api:
     enabled: true
   ```

   ```yaml
   # compose.yaml
   services:
     routerctl:
       image: ghcr.io/eduardpaul/routerctl:latest
       command: ["watch", "-c", "/config/routerctl.yaml"]
       environment:
         ROUTERCTL_HTTP_TOKEN_FILE: /run/secrets/routerctl_token
       volumes: ["./routerctl.yaml:/config/routerctl.yaml:ro", "routerctl-data:/data"]
       secrets: [router_password, routerctl_token]
     nodered:
       image: nodered/node-red:5.0.9
       ports: ["1880:1880"]
   ```

2. In Node-RED, go to **Manage palette → Install** and search for
   `@eduardpaul/node-red-routerctl`.
3. Add a **routerctl-server** config node with URL `http://routerctl:9100`
   and the same token.

Requires Node-RED 4 or newer (tested on 5.0 and 4.1).

## Nodes

| Node | Use it to |
|---|---|
| **event** | get events: `device_new`, `device_online` / `device_offline`, `wan_down` / `wan_up`, `wan_ip_changed`, `firmware_update`, `node_*`, `action_done`. Filter by type, router, MAC or unknown-only. Presence mode outputs `home` / `not_home` |
| **query** | read clients, router state, mesh nodes, actions, native sources or the inventory |
| **action** | run a router action (the editor lists them). `msg.payload` may be an object (`{"mac": "...", "state": "on"}`), `"on"` / `"k=v"`, `true` / `false`, or a routerctl event (its MAC is used) |
| **inventory** | approve, unapprove, label, forget or get a device. Wire a `device_new` event straight in |
| **server** | the daemon's URL and token, shared by all nodes |

An event's `msg.payload` is the routerctl event, and `msg.payload.summary`
is ready to use in a notification:

```json
{"id": "3a7abc31-42", "type": "device_new", "device": "router",
 "summary": "New device on router: visitor (02:00:00:00:20:06), IP 192.168.50.99",
 "data": {"mac": "02:00:00:00:20:06", "name": "visitor", "known": false}}
```

A refused or failed action raises an error for a **Catch** node, with the
code in `msg.payload.error.code` (`forbidden` when the daemon's policy
refuses it, e.g. reboot).

## Examples

**Import → Examples → @eduardpaul/node-red-routerctl**:

1. Intruder alert with approve link
2. Presence to automations
3. Bedtime internet pause
4. Reconnect a stuck internet uplink
5. Daily network report

## Good to know

- The event stream reconnects on its own and resumes where it left off.
  The daemon keeps the last 500 events.
- The token is a Node-RED credential and never reaches the browser.
- The daemon decides which actions may run (`commands.max_danger`), not
  the flow.
- Without these nodes, use the built-in MQTT nodes: see
  [Home Assistant topics](https://github.com/eduardpaul/routerctl/blob/main/docs/home-assistant.md#topics).

## Development

```sh
npm install
npm test                 # unit tests
npm run test:contract    # the nodes in Node-RED against a real routerctl (needs Go)
npm run test:docker      # Docker: routerctl + Node-RED 5.0 and 4.1
```

The contract tests check every payload against
`routerctl/internal/sink/api/testdata/contract.golden.json`.

## License

Apache-2.0
