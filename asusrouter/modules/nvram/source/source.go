// Package source mirrors the fetching part of
// asusrouter/modules/nvram/source.py: `fetch_state`, the async helpers and
// the registration. The indexed source and the translation stay in
// package nvram so the device identity can use them (import cycle).
package source

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// fetchState fetches the NVRAM data state.
func fetchState(ctx context.Context, c *registry.Context, srcs []source.DataSource) (map[source.ID]any, error) {
	items := nvram.AsItems(srcs)

	// Map each raw key back to the item that requested it (in request
	// order; a later duplicate key replaces the earlier requester)
	forward := map[string]nvram.ARNvramItem{}
	var order []string
	for _, item := range items {
		key := nvram.ResolveKey(item)
		if _, seen := forward[key]; !seen {
			order = append(order, key)
		}
		forward[key] = item
	}

	hooks := make([]endpoint.ARHookItem, 0, len(order))
	for _, key := range order {
		hooks = append(hooks, forward[key])
	}
	request := endpoint.HookRequest(hooks...)
	raw, err := c.Read(ctx, endpoint.AREndpointFetchData, request)
	if err != nil {
		return nil, err
	}

	response, ok := raw.(map[string]any)
	if !ok {
		return map[source.ID]any{}, nil
	}

	result := map[source.ID]any{}
	for key, value := range response {
		if requester, ok := forward[key]; ok {
			result[source.IDOf(requester)] = value
		}
	}
	return result, nil
}

// AsyncFetchValues fetches NVRAM values through the data pipeline, or {}
// if unavailable. The request is a single item, a slice of items or a
// collection; the result is keyed by item ID.
func AsyncFetchValues(ctx context.Context, c *registry.Context, request any, kw registry.Kwargs) (map[source.ID]any, error) {
	if c == nil || c.FetchDataMulti == nil {
		return map[source.ID]any{}, nil
	}
	items := nvram.AsItems(request)
	srcs := make([]source.DataSource, len(items))
	for i, item := range items {
		srcs[i] = item
	}
	values, err := c.FetchDataMulti(ctx, srcs, kw)
	if err != nil {
		return nil, err
	}
	if values == nil {
		return map[source.ID]any{}, nil
	}
	return values, nil
}

// AsyncGetValue fetches a single NVRAM value through the data pipeline,
// or nil.
func AsyncGetValue(ctx context.Context, c *registry.Context, item nvram.ARNvramItem, kw registry.Kwargs) (any, error) {
	values, err := AsyncFetchValues(ctx, c, item, kw)
	if err != nil {
		return nil, err
	}
	return values[source.IDOf(item)], nil
}

// AsyncExpireValues expires cached NVRAM values so the next fetch
// refetches them.
//
// NVRAM items are cached per item; a module expiring its own source must
// expire the underlying items too, or a refetch within the cache window
// returns the stale pre-action values.
func AsyncExpireValues(ctx context.Context, expire func(ctx context.Context, src source.DataSource), request any) {
	if expire == nil {
		return
	}
	for _, item := range nvram.AsItems(request) {
		expire(ctx, item)
	}
}

// translateState mirrors `translate_state` (multi).
func translateState(data map[source.ID]any, tc *registry.TranslateContext) map[source.ID]any {
	return nvram.TranslateState(data)
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:           "ARNvramType",
		FetchMulti:     fetchState,
		TranslateMulti: translateState,
	})
	registry.RegisterSource(registry.SourceEntry{
		Name:           "ARNvramIndexSource",
		FetchMulti:     fetchState,
		TranslateMulti: translateState,
	})
}
