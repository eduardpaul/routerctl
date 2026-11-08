# routerctl

[![CI](https://github.com/eduardpaul/routerctl/actions/workflows/ci.yml/badge.svg)](https://github.com/eduardpaul/routerctl/actions/workflows/ci.yml)

A single static binary to read and control home routers, and a daemon
that turns their state into events for Home Assistant, Node-RED,
notifications and Prometheus.

Supported devices:

| Brand | Driver | Read | Actions |
|---|---|---|---|
| ASUS (ASUSWRT, AiMesh) | `asus` | yes | LED, WiFi, block clients, speedtest, VPN, reboot, ... |
| ZTE ZXHN F86xx fiber ONTs | `zte` | yes | none (read-only) |

## Install

- **Docker:** `docker pull ghcr.io/eduardpaul/routerctl` (linux/amd64,
  arm64, arm/v7; see [Run in Docker](#run-in-docker)).
- **Binary:** download it from
  [Releases](https://github.com/eduardpaul/routerctl/releases) (Linux
  amd64/arm64/armv7, macOS Intel/Apple Silicon, Windows amd64/arm64), or
  build it with `make build` (Go 1.24+). The binaries are not signed:
  - **macOS:** if a browser download is blocked ("cannot be opened"), run
    `xattr -d com.apple.quarantine routerctl` once.
  - **Windows:** SmartScreen may warn on first start; choose **More info →
    Run anyway**.
- **Go:** `go install github.com/eduardpaul/routerctl/routerctl/cmd/routerctl@latest`.

## Quick start

```sh
export ROUTERCTL_HOST=192.168.50.1 ROUTERCTL_USERNAME=admin
read -rs ROUTERCTL_PASSWORD; export ROUTERCTL_PASSWORD

routerctl detect 192.168.50.1     # which brand is it? (no login)
routerctl status                  # identity, WAN, CPU, RAM, firmware
routerctl clients --online        # who is connected
routerctl do                      # list the actions of this router
routerctl do led off              # run one
```

Add `-o json` or `-o csv` to any read command.

## Common tasks

| I want to... | Do this |
|---|---|
| Configure several routers | [docs/configure.md](docs/configure.md) |
| Be alerted when an unknown device joins | [docs/watch.md](docs/watch.md#get-alerts) |
| See routers and presence in Home Assistant | [docs/home-assistant.md](docs/home-assistant.md) |
| Automate with Node-RED | [node-red/README.md](node-red/README.md) |
| Script against a local API | [docs/http-api.md](docs/http-api.md) |
| Scrape metrics with Prometheus | [docs/watch.md](docs/watch.md#export-metrics) |
| Read raw brand data | `routerctl sources`, then `routerctl get <source>` |
| Build, test or add a brand | [docs/development.md](docs/development.md) |

## Run in Docker

```yaml
# compose.yaml
services:
  routerctl:
    image: ghcr.io/eduardpaul/routerctl:latest
    restart: unless-stopped
    command: ["watch", "-c", "/config/routerctl.yaml"]
    volumes:
      - ./routerctl.yaml:/config/routerctl.yaml:ro
      - routerctl-data:/data          # device inventory
    secrets: [router_password]
secrets:
  router_password: {file: ./secrets/router_password}
volumes:
  routerctl-data:
```

Create the config with `docker run --rm ghcr.io/eduardpaul/routerctl config example > routerctl.yaml`
and edit it. The image runs as a non-root user and listens on `:9100`.

## Credits and license

routerctl is licensed under the [Apache License 2.0](LICENSE).

The ASUS support is a Go port of the
[asusrouter](https://github.com/Vaskivskyi/asusrouter) Python library by
Yevhenii Vaskivskyi (Apache-2.0), kept in parity with it
([asusrouter/PARITY.md](asusrouter/PARITY.md)). See [NOTICE](NOTICE).

This project is not affiliated with or endorsed by ASUSTeK Computer Inc.,
ZTE Corporation or the asusrouter project. ASUS, ASUSWRT, ZTE and ZXHN are
trademarks of their owners.
