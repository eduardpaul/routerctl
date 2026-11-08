// Package registry mirrors asusrouter/registry.py.
//
// Python resolves callables by class MRO; Go registers each concrete
// source/action name explicitly (a subclass that inherits its callables in
// Python registers the same functions under its own name in Go).
//
// Python passes **kwargs to fetchers/translators; Go passes a context
// struct with the well-known callbacks as typed fields and the remaining
// keyword arguments in Kwargs.
package registry

import (
	"context"
	"sort"
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
)

// Kwargs carries Python keyword arguments that have no typed field
// (for example "refresh", "save", "force").
type Kwargs map[string]any

// Bool reads a boolean keyword argument (missing or non-bool is false).
func (k Kwargs) Bool(name string) bool {
	if k == nil {
		return false
	}
	b, _ := k[name].(bool)
	return b
}

// ReadFunc is `async_read`: fetch and parse an endpoint.
type ReadFunc func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error)

// FetchRawFunc is `async_fetch` / `fetch_raw_callback`: the unparsed body,
// nil when the endpoint is unavailable.
type FetchRawFunc func(ctx context.Context, ep endpoint.AREndpoint, request string) (*string, error)

// FetchDataFunc is `fetch_data_callback`. Python returns `{source:
// content}`; Go returns the content of that one source directly (nil when
// there is none). kw may be nil.
type FetchDataFunc func(ctx context.Context, src source.DataSource, kw Kwargs) (any, error)

// FetchDataMultiFunc is `fetch_data_callback` called with a collection
// (e.g. `DEVICE_REQUEST`): the content of every requested source that has
// one, keyed by source ID. Multicaller sources requested together are
// fetched in one batched call, like Python. kw may be nil.
type FetchDataMultiFunc func(ctx context.Context, srcs []source.DataSource, kw Kwargs) (map[source.ID]any, error)

// Context is what Python passes as `callback` plus `**kwargs` to
// `fetch_state`, `run_action` and `async_probe_state`.
type Context struct {
	// Read is the positional `callback` (`async_read`)
	Read ReadFunc
	// FetchRaw is `fetch_raw_callback`
	FetchRaw FetchRawFunc
	// FetchData is `fetch_data_callback`
	FetchData FetchDataFunc
	// FetchDataMulti is `fetch_data_callback` with a collection
	FetchDataMulti FetchDataMultiFunc
	// RunAction is `run_action_callback`
	RunAction action.RunActionFunc
	// Identity is the `identity` keyword (never nil from the router)
	Identity *device.ARDeviceIdentity
	// ConnectionConfig is the `connection_config` keyword
	ConnectionConfig *config.ARConnectionConfig

	// Action-only callbacks
	Expire         func(ctx context.Context, src source.DataSource)
	CredentialsGet func() (username, password string)
	CredentialsSet func(ctx context.Context, username, password string) (bool, error)

	// Remaining keyword arguments
	Kwargs Kwargs
}

// TranslateContext is the keyword arguments of `translate_state` /
// `translate_action`.
type TranslateContext struct {
	Identity *device.ARDeviceIdentity
	// Previous is the `previous` keyword (the last committed content)
	Previous any
	Kwargs   Kwargs
}

// FetchFunc is a single-source `fetch_state`.
type FetchFunc func(ctx context.Context, c *Context, src source.DataSource) (any, error)

// FetchMultiFunc is a multicaller `fetch_state` (`multi=True`): one
// batched request for several sources, keyed by source ID.
type FetchMultiFunc func(ctx context.Context, c *Context, srcs []source.DataSource) (map[source.ID]any, error)

// TranslateFunc is a single-value `translate_state`.
type TranslateFunc func(raw any, tc *TranslateContext) any

// TranslateMultiFunc is a batch `translate_state` (`multi=True`).
type TranslateMultiFunc func(data map[source.ID]any, tc *TranslateContext) map[source.ID]any

// RunActionFunc is `run_action`.
type RunActionFunc func(ctx context.Context, c *Context, a action.ARAction) (any, error)

// TranslateActionFunc is `translate_action`.
type TranslateActionFunc func(raw any, tc *TranslateContext) any

// SourceEntry is one `register_source` call.
type SourceEntry struct {
	// Name is the Python class name the entry is registered for
	Name string
	// Default builds the default instance used by `_default_sources`
	// (nil when the source cannot be built without arguments)
	Default func() source.DataSource

	Fetch          FetchFunc
	FetchMulti     FetchMultiFunc
	Translate      TranslateFunc
	TranslateMulti TranslateMultiFunc
}

// Multi reports whether the fetch is a multicaller.
func (e *SourceEntry) Multi() bool { return e.FetchMulti != nil }

// ActionEntry is one `register_action` call.
type ActionEntry struct {
	Name      string
	Run       RunActionFunc
	Translate TranslateActionFunc
}

var (
	mu      sync.RWMutex
	sources = map[string]*SourceEntry{}
	actions = map[string]*ActionEntry{}
)

// RegisterSource mirrors `register_source` (call it from `init`).
func RegisterSource(e SourceEntry) {
	mu.Lock()
	defer mu.Unlock()
	entry := e
	sources[e.Name] = &entry
}

// RegisterAction mirrors `register_action` (call it from `init`).
func RegisterAction(e ActionEntry) {
	mu.Lock()
	defer mu.Unlock()
	entry := e
	actions[e.Name] = &entry
}

// Source returns the entry registered for a source, or nil.
func Source(src source.DataSource) *SourceEntry {
	if src == nil {
		return nil
	}
	return SourceByName(src.SourceName())
}

// SourceByName returns the entry registered under a Python class name.
func SourceByName(name string) *SourceEntry {
	mu.RLock()
	defer mu.RUnlock()
	return sources[name]
}

// Action returns the entry registered for an action, or nil.
func Action(a action.ARAction) *ActionEntry {
	if a == nil {
		return nil
	}
	mu.RLock()
	defer mu.RUnlock()
	return actions[a.ActionName()]
}

// SourceNames lists every registered source name, sorted.
func SourceNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(sources))
	for n := range sources {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ActionNames lists every registered action name, sorted.
func ActionNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(actions))
	for n := range actions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// DefaultSources mirrors `AsusRouter._default_sources`: the default
// instance of every fetchable source, by name.
func DefaultSources() []source.DataSource {
	var out []source.DataSource
	for _, name := range SourceNames() {
		e := SourceByName(name)
		if e.Default == nil || (e.Fetch == nil && e.FetchMulti == nil) {
			continue
		}
		out = append(out, e.Default())
	}
	return out
}
