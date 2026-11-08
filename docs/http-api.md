# Use the HTTP API

The daemon serves a local REST API with a live event stream. The Node-RED
nodes and `routerctl --via` use it.

## Turn it on

```yaml
api:
  enabled: true
http:
  listen: "127.0.0.1:9100"          # local only: no token needed
  # listen: ":9100"                 # reachable from the network: a token is required
  # token_file: /run/secrets/routerctl_token
```

Send the token as `Authorization: Bearer <token>`. The daemon refuses to
start when the API listens beyond localhost without a token.

```sh
curl -s localhost:9100/api/v1/devices/asus/clients?online=true
curl -s -X POST localhost:9100/api/v1/devices/asus/actions/led -d off
curl -sN localhost:9100/api/v1/events?types=device_new
```

## Endpoints

| Method | Path | Returns |
|---|---|---|
| GET | `/api/v1/info` | version, devices, event types, command policy |
| GET | `/api/v1/devices` | all routers |
| GET | `/api/v1/devices/{dev}` | one router: identity, capabilities, state |
| GET | `/api/v1/devices/{dev}/clients?online=&known=` | clients with presence and inventory data |
| GET | `/api/v1/devices/{dev}/nodes` | mesh nodes |
| GET | `/api/v1/devices/{dev}/actions` | actions, with `allowed` and `reason` from the policy |
| POST | `/api/v1/devices/{dev}/actions/{action}` | runs it. Body: JSON args, `on`, or `k=v`. Optional `X-Request-Id` header |
| GET | `/api/v1/devices/{dev}/sources` | native (brand) sources |
| GET | `/api/v1/devices/{dev}/sources/{name}` | one native source |
| GET | `/api/v1/inventory?known=&online=&device=` | device inventory |
| PATCH | `/api/v1/inventory/{mac}` | body `{"label": "...", "known": true}` |
| DELETE | `/api/v1/inventory/{mac}` | forget the device |
| GET | `/api/v1/events?types=&devices=&macs=&unknown=&state=` | event stream (Server-Sent Events) |

- Errors are `{"error": {"code": "...", "message": "..."}}`. The codes
  include `invalid_args`, `forbidden`, `unsupported`, `unknown_device`,
  `not_found`, `not_ready`, `rejected`, `unreachable` and `timeout`.
- The event stream resumes from `Last-Event-ID`; the daemon keeps the last
  500 events. Add `state=1` to also get a `state` frame after every poll.
- The JSON shapes are fixed by
  `routerctl/internal/sink/api/testdata/contract.golden.json`.

## Use the running daemon from the CLI

`do`, `clients` and `devices` can go through the daemon instead of
logging in to the router:

```sh
export ROUTERCTL_VIA=http://127.0.0.1:9100    # or --via
routerctl clients
routerctl do wifi band=5g1 state=off           # the daemon's policy applies
routerctl devices approve aa:bb:cc:dd:ee:ff
```

Use this for routers that allow only one session (ZTE). Without `--via`,
the CLI would compete with the daemon for that session. For a token, set
`ROUTERCTL_HTTP_TOKEN` or pass `--token-file`.
