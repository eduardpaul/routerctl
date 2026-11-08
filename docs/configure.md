# Configure routerctl

## Create a config file

```sh
routerctl config example > routerctl.yaml   # annotated example, edit it
routerctl config check -c routerctl.yaml    # validate (drivers, options, secrets)
routerctl config show -c routerctl.yaml     # effective config, secrets masked
```

routerctl finds the file through `-c`, `ROUTERCTL_CONFIG`, or the first
of these:

| System | Search order |
|---|---|
| Linux | `./routerctl.yaml`, `~/.config/routerctl/config.yaml`, `/etc/routerctl/config.yaml` |
| macOS | `./routerctl.yaml`, `~/Library/Application Support/routerctl/config.yaml`, `/etc/routerctl/config.yaml` |
| Windows | `.\routerctl.yaml`, `%AppData%\routerctl\config.yaml` |

Settings apply in this order: defaults, then
the file, then `ROUTERCTL_*` env vars, then flags.

## Add a router

```yaml
devices:
  - name: asus                # used in topics, events and -d asus
    driver: auto              # auto | asus | zte | fake
    host: 192.168.50.1
    username: admin
    password_file: /run/secrets/asus_password
```

- `driver: auto` detects the brand from the home page. If detection is
  ambiguous, set the driver (`routerctl detect -v <host>` shows the
  candidates).
- Pick a device with `-d <name>`. With a single device, `-d` is optional.
- `intervals:` sets the poll interval per part (`clients`, `wan`,
  `system`, `traffic`, `nodes`, `firmware`). `0` disables a part.

Driver options go under `options:`:

| Driver | Option | Meaning |
|---|---|---|
| `asus` | `cache` (duration, default `5s`) | how long the library reuses data |
| `zte` | `insecure` (bool) | accept any HTTPS certificate |
| `zte` | `min_interval` (duration) | spacing between requests and polls |
| `zte` | `pon` (bool, default `true`) | read the PON optics |
| `fake` | `demo` (bool) | change state on a schedule (for trying things out) |

## Keep secrets out of the file

- Use `password_file` / `token_file` (for example Docker secrets) or env
  vars. Inline values work but are discouraged.
- On the command line, pass the password with `ROUTERCTL_PASSWORD` or
  `--password-file`, not `--password` (it shows in `ps`).
- Secrets are never logged, and `config show` masks them.

## Configure one router with env vars only

```sh
ROUTERCTL_HOST=192.168.50.1 ROUTERCTL_USERNAME=admin \
ROUTERCTL_PASSWORD_FILE=/run/secrets/asus_password routerctl watch
```

| Area | Variables |
|---|---|
| Device | `ROUTERCTL_HOST`, `_USERNAME`, `_PASSWORD`, `_PASSWORD_FILE`, `_DRIVER`, `_SSL`, `_PORT`, `_PIN_SHA256`, `_NAME` |
| General | `ROUTERCTL_CONFIG`, `_STATE_FILE`, `_LOG_LEVEL`, `_LOG_FORMAT`, `_OFFLINE_GRACE` |
| MQTT | `ROUTERCTL_MQTT_BROKER` (turns MQTT on), `_MQTT_USERNAME`, `_MQTT_PASSWORD(_FILE)`, `_MQTT_BASE_TOPIC`, `_MQTT_DISCOVERY`, `_MQTT_DISCOVERY_PREFIX`, `_MQTT_TRACKERS`, `_MQTT_MAX_DANGER` |
| HTTP, API, metrics | `ROUTERCTL_HTTP_LISTEN`, `_HTTP_TOKEN(_FILE)`, `_API_ENABLED`, `_API_MAX_DANGER`, `_METRICS_ENABLED` |
| Actions | `ROUTERCTL_COMMANDS_MAX_DANGER` |
| Notifications | `ROUTERCTL_NTFY_URL` / `_TOPIC` / `_TOKEN(_FILE)` / `_EVENTS`, `ROUTERCTL_GOTIFY_URL` / `_TOKEN(_FILE)` / `_EVENTS`, `ROUTERCTL_WEBHOOK_URL` / `_EVENTS` |
| Output | `ROUTERCTL_STDOUT` |

## Choose what automations may do

Every action has a danger level: `safe` (LED, speedtest), `disruptive`
(WiFi off, blocking a client, VPN) or `destructive` (reboot).

```yaml
commands:
  max_danger: disruptive      # default: everything except reboot & co.
  # allow: [led, wifi, block-client]   # only these
  # deny: [reconnect-wan]              # never these
mqtt:
  max_danger: safe            # can only be stricter than commands
api:
  max_danger: disruptive
```

- This policy applies to MQTT, the HTTP API and Node-RED.
- On the command line, `routerctl do` runs anything up to `disruptive`;
  `destructive` actions need `--yes`.

## Pin a self-signed certificate

```yaml
devices:
  - name: asus
    host: 192.168.50.1
    ssl: true
    pin_sha256: "AB:CD:..."   # SHA-256 of the router's certificate
```

With a pin set, routerctl never falls back to plain HTTP.
