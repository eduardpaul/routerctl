# Develop routerctl

## Set up

```sh
git clone --recurse-submodules https://github.com/eduardpaul/routerctl
cd routerctl
make build test              # Go 1.24+; bin/routerctl
```

You also need `uv` for the Python parity tools, and Node.js 18+ plus
Docker for the Node-RED package.

## Build and test

| Command | Does |
|---|---|
| `make build` | `bin/routerctl` (static, `CGO_ENABLED=0`) |
| `make test` | every module with `-race` (`make test RACE=` without it), arch tests included |
| `make fmt vet` | gofmt check and vet on every module |
| `make cross` | `bin/routerctl-linux-{amd64,arm64,armv7}` |
| `make docker` | the distroless image `routerctl:latest` |
| `make build TAGS=noasus,nozte` | leave brands out (`fake` is always in) |
| `make gen` | regenerate the Go enums and the source registry from Python |
| `make parity` | compare Python and Go on the recorded sessions |
| `make lint-py` | ruff on the Python tooling |
| `make licenses` | check that the LICENSE and NOTICE copies match |

`./...` does not cross modules at the repository root. To test inside one
module, run `cd routerkit && go test ./...`, or pass a directory:
`go test ./routerkit/drivers/zte`.

For the Node-RED package, see [node-red/README.md](../node-red/README.md#development).

## Repository layout

| Path | What |
|---|---|
| `netkit/` | module: TLS pinning (`tlspin`), HTTP record and replay (`httprec`). Stdlib only |
| `asusrouter/` | module: the Go port of the Python library, file by file ([PARITY.md](../asusrouter/PARITY.md)) |
| `routerkit/` | module: the driver contract, brand detection, brand libraries (`brands/zte`), drivers (`asus`, `zte`, `fake`), the conformance suite (`drivertest`) |
| `routerctl/` | module: the CLI and daemon (`cmd/routerctl`, everything else `internal/`) |
| `parity/` | Python/Go parity harness, generators, sanitized fixtures |
| `third_party/asusrouter-py/` | the Python library (git submodule, pinned) |
| `node-red/` | the Node-RED package |

## Import rules

`arch_test.go`, the same file in every module, enforces these:

| Package | May import |
|---|---|
| `netkit` | stdlib only |
| `routerkit/proto/*` | `netkit`, `proto` |
| brand libraries (`routerkit/brands/<b>`, `asusrouter`) | `netkit`, `proto`, their own tree, external libraries |
| `routerkit/drivers/<b>` | the core, `detect`, **one** brand library, `proto`, `netkit`; `drivertest` in tests |
| core (`routerkit` root) | stdlib only |
| `routerctl/internal/service/*` | the core, other services |
| `routerctl/internal/sink/*` | the core, services, other sinks |
| `routerctl/internal/config` | the core |
| `routerctl/internal/app`, `cmd/routerctl` | everything above |

Further rules:

- Libraries never register themselves with `init()`. Only `main` does, in
  `routerctl/cmd/routerctl/brand_<name>.go`.
- Services and sinks never read `Extra` maps: anything they need is a
  typed field.
- Every transport (CLI, MQTT, API) runs actions through
  `service/commands` and publishes the `service/view` payloads.

## Add a brand

Use ZTE as the worked example.

1. **Library:** `routerkit/brands/<brand>/`. It speaks the brand's own
   vocabulary and never imports `routerkit`. Write the protocol notes in
   `PROTOCOL.md`.
2. **Captures:** record into `routerkit/brands/<brand>/testdata/local/`,
   which is git-ignored. Sanitize the session and scan it for leaks
   before committing anything derived from it (see the ZTE `scripts/` and
   [PROTOCOL.md](../routerkit/brands/zte/PROTOCOL.md#captures-and-fixtures)).
3. **Fake:** `routerkit/brands/<brand>/<brand>fake/`, an `httptest` server
   with embedded fixtures and the sanitized session.
4. **Driver:** `routerkit/drivers/<brand>/`.
   - Embed `routerkit.Base` and export `Factory()` (Info, Options, New).
   - Declare exact `Capabilities` after `Connect`.
   - Use catalog actions (`routerkit.Std`), and call `BindInvocation`
     first in `Do`.
   - Classify errors (`ErrAuth`, `ErrUnreachable`, `ErrBusy`, ...). Never
     put the password in an error or a log.
5. **Fingerprints:** unauthenticated GET probes only. Add the home pages to
   `routerkit/detect/testdata/homepages/`; detection must stay
   unambiguous.
6. **Conformance:** `conformance_test.go` runs `drivertest.Run` over the
   fake and the recorded session. `live_test.go` runs against a real
   device with `ROUTERCTL_LIVE=<driver>` (read-only). Create the golden
   with `go test ./routerkit/drivers/<brand> -drivertest.update` and
   review the diff.
7. **CLI:** add `routerctl/cmd/routerctl/brand_<brand>.go` with
   `//go:build !no<brand>` and
   `func init() { brands = append(brands, <brand>driver.Factory()) }`.
8. **Check:** `make fmt vet test build`, `make build TAGS=no<brand>`,
   `make docker`.
9. **Docs:** add the brand to the README table and its options to
   [configure.md](configure.md).

## Keep the ASUS port in sync with Python

See [asusrouter/PARITY.md](../asusrouter/PARITY.md#sync-with-the-python-library).

## Change the HTTP API

The API payloads are a contract shared with the Node-RED package:

1. Change the Go types.
2. `cd routerctl && go test ./internal/sink/api -run TestContract -update`
3. Review the diff of `testdata/contract.golden.json`.
4. Run `npm run test:contract` in `node-red/`.

## Release

Release the modules in dependency order, netkit → asusrouter → routerkit →
routerctl, and only the ones that changed. For each module:

1. Require the released siblings, outside the workspace (netkit has none):

   ```sh
   cd routerkit
   GOWORK=off go get github.com/eduardpaul/routerctl/netkit@v0.1.0 \
                     github.com/eduardpaul/routerctl/asusrouter@v0.1.0
   GOWORK=off go mod tidy
   ```

2. Check it the way a consumer builds it:

   ```sh
   GOWORK=off go build ./... && GOWORK=off go test ./... && GOWORK=off go mod tidy -diff
   ```

3. Commit the `go.mod` and `go.sum` changes, then tag and push the tag:

   ```sh
   git tag -a routerkit/v0.1.0 -m "routerkit v0.1.0"
   git push origin routerkit/v0.1.0
   ```

4. Wait for the **Release** workflow to pass before you tag the next
   module, because the next one requires this tag.

What a tag triggers:

| Tag | Release workflow |
|---|---|
| `netkit/v*`, `asusrouter/v*`, `routerkit/v*` | the module is built and tested with `GOWORK=off` |
| `routerctl/v*` | the same, then `go install` at the tag, Linux binaries on a GitHub release, and the image `ghcr.io/eduardpaul/routerctl:{X.Y.Z,X.Y,latest}` |
| `node-red/v*` | unit tests, then `npm publish --provenance` (the tag must match `package.json`) |

- For `asusrouter`, name the Python version and commit in the tag message.
- npm publishing needs the `NPM_TOKEN` secret or, preferably, a trusted
  publisher on npmjs.com (repository `eduardpaul/routerctl`, workflow
  `release.yml`).

## CI

| Workflow | Runs | Checks |
|---|---|---|
| `ci.yml` | push to `main`, PRs, weekly | Go fmt, vet, race tests, build, licenses (on the minimum Go and on stable); parity with the pinned Python; Docker image; Node-RED unit, contract and Docker end-to-end tests |
| `release.yml` | release tags | see the table above |
| `release.yml` (dry run) | PRs that change a release input, manual runs | builds the archives, the multi-arch image and the npm package; publishes nothing |
| `upstream.yml` | weekly | parity against the newest Python `dev`, as an early warning (report only) |

Dependabot opens weekly PRs for actions, Go modules, npm, uv, the Docker
base images and the Python submodule. A submodule bump fails parity until
its Go port is added to the same PR.
