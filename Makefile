# routerctl: build, test and static cross-compiles (CGO_ENABLED=0).
#
# The repository root is a workspace (go.work) of four modules: netkit,
# asusrouter, routerkit and routerctl. At the root `./...` does not cross
# module boundaries, so the targets name each module's packages by its
# module path: `go list -m` lists the modules of go.work.

BIN      := routerctl
PKG      := ./routerctl/cmd/routerctl
MODULES  := $(shell go list -m)
PKGS     := $(addsuffix /...,$(MODULES))
VERSION  ?= $(shell git describe --tags --match 'routerctl/v*' --always --dirty 2>/dev/null | sed 's#^routerctl/##' || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
# Extra build tags, e.g. TAGS=noasus or TAGS=nozte
TAGS     ?=
# The race detector needs cgo; `make test RACE=` runs without it
RACE     ?= -race
GOBUILD  := CGO_ENABLED=0 go build -trimpath -tags '$(TAGS)' -ldflags '$(LDFLAGS)'
IMAGE    ?= routerctl
PLATFORMS := linux/amd64,linux/arm64,linux/arm/v7

.PHONY: all build test vet race fmt modules cross docker docker-multiarch clean gen parity lint-py licenses

all: fmt vet test build

build:
	$(GOBUILD) -o bin/$(BIN) $(PKG)

# Every module, the arch tests included
test:
	go test $(RACE) -count=1 $(PKGS)

race:
	go test -race -count=1 $(PKGS)

vet:
	go vet $(PKGS)

# Fails when gofmt would change a file (and lists them)
fmt:
	@test -z "$$(gofmt -l $$(go list -m -f '{{.Dir}}') | tee /dev/stderr)"

# The workspace modules, one per line
modules:
	@printf '%s\n' $(MODULES)

# Static binaries: bin/routerctl-linux-amd64, -linux-arm64, -linux-armv7
cross:
	GOOS=linux GOARCH=amd64        $(GOBUILD) -o bin/$(BIN)-linux-amd64 $(PKG)
	GOOS=linux GOARCH=arm64        $(GOBUILD) -o bin/$(BIN)-linux-arm64 $(PKG)
	GOOS=linux GOARCH=arm GOARM=7  $(GOBUILD) -o bin/$(BIN)-linux-armv7 $(PKG)
	GOOS=darwin GOARCH=amd64       $(GOBUILD) -o bin/$(BIN)-darwin-amd64 $(PKG)
	GOOS=darwin GOARCH=arm64       $(GOBUILD) -o bin/$(BIN)-darwin-arm64 $(PKG)
	GOOS=windows GOARCH=amd64      $(GOBUILD) -o bin/$(BIN)-windows-amd64.exe $(PKG)
	GOOS=windows GOARCH=arm64      $(GOBUILD) -o bin/$(BIN)-windows-arm64.exe $(PKG)

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg REVISION=$(shell git rev-parse HEAD) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-multiarch:
	docker buildx build --platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) --build-arg REVISION=$(shell git rev-parse HEAD) -t $(IMAGE):$(VERSION) .

clean:
	rm -rf bin

# ---- Python parity (needs uv and the submodule: git submodule update --init)
PY := uv run --project parity python

# Regenerate the Go enums and the source registry from the Python library
gen:
	$(PY) parity/gen_go_enums.py
	$(PY) parity/gen_go_register.py

# Replay the recorded sessions through Python and Go and compare
parity:
	parity/check.sh

lint-py:
	uv run --project parity ruff check parity routerkit/brands
	uv run --project parity ruff format --check parity routerkit/brands

# The license and notice copies match the root ones
licenses:
	for d in netkit asusrouter routerkit routerctl node-red; do cmp LICENSE $$d/LICENSE; done
	cmp NOTICE asusrouter/NOTICE
