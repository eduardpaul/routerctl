// Package count mirrors asusrouter/modules/clients/count/ (`__init__.py`
// only re-exports source.py): the connected clients count data source.
package count

import (
	"context"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/clients"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

// Diagnostics window: newest of several points; 60/30 matches the device
const (
	diagDuration = 60
	diagPoint    = 30
)

var diagRequestType = endpoint.GetEndpointRequestType(endpoint.AREndpointFetchDiagnosticsActiveClient)

// Don't count AI board (starts with prefix) when counting online clients
const aiboardNamePrefix = "aiboard"

// ARClientsCountSource mirrors `ARClientsCountSource`: connected clients
// count data source, optionally for one target node.
type ARClientsCountSource struct {
	target *identifiers.MacAddress
}

// NewARClientsCountSource mirrors `ARClientsCountSource(target)`.
func NewARClientsCountSource(target any) *ARClientsCountSource {
	s := &ARClientsCountSource{}
	s.SetTarget(target)
	return s
}

// SourceName returns the Python class name.
func (*ARClientsCountSource) SourceName() string { return "ARClientsCountSource" }

// Target gets the target MAC address.
func (s *ARClientsCountSource) Target() *identifiers.MacAddress { return s.target }

// SetTarget sets the target MAC address.
func (s *ARClientsCountSource) SetTarget(value any) {
	s.target = identifiers.MacAddressFromValueSafe(value)
}

// SourceKey mirrors `_key`: key by the target MAC.
func (s *ARClientsCountSource) SourceKey() []any {
	if s.target == nil {
		return []any{nil}
	}
	return []any{*s.target}
}

// ARClientsCountSourceUniversal is the universal instance - preferred
// (targets all nodes)
var ARClientsCountSourceUniversal = &ARClientsCountSource{}

// timeNow is the clock of the request timestamp (a seam for tests).
var timeNow = time.Now

// buildRequest mirrors `_build_request`: build the active-client
// diagnostics request for one node MAC.
func buildRequest(mac identifiers.MacAddress) string {
	return writers.DictToRequest([]writers.KeyValue{
		{Key: "ts", Value: int(timeNow().Unix())},
		{Key: "duration", Value: diagDuration},
		{Key: "point", Value: diagPoint},
		{Key: "node_mac", Value: mac.AsAsus()},
	}, diagRequestType)
}

// fetchModernOne mirrors `_fetch_modern_one`: fetch the newest
// connected-client count for one node.
func fetchModernOne(ctx context.Context, c *registry.Context, mac identifiers.MacAddress) (*int, error) {
	data, err := c.Read(ctx, endpoint.AREndpointFetchDiagnosticsActiveClient, buildRequest(mac))
	if err != nil {
		return nil, err
	}
	var counts []any
	if m, ok := data.(map[string]any); ok {
		counts, _ = m["count"].([]any)
	}
	if len(counts) == 0 {
		return nil, nil
	}

	// A refresh can momentarily report 0; fall back one point only
	last := converters.RawToInt(counts[len(counts)-1])
	if last != nil && *last == 0 && len(counts) >= 2 {
		if prev := converters.RawToInt(counts[len(counts)-2]); prev != nil && *prev != 0 {
			return prev, nil
		}
	}

	return last, nil
}

// isAiboard mirrors `_is_aiboard`: whether the client is the device's own
// AI board.
func isAiboard(client clients.ARClient) bool {
	return client.Name != nil && strings.HasPrefix(strings.ToLower(*client.Name), aiboardNamePrefix)
}

// targetMacs mirrors `_target_macs`: resolve the nodes to report: the
// target, else all known nodes.
func targetMacs(src *ARClientsCountSource, identity *device.ARDeviceIdentity) []identifiers.MacAddress {
	if src.Target() != nil {
		return []identifiers.MacAddress{*src.Target()}
	}
	if nodes := identity.Aimesh().Macs(); len(nodes) > 0 {
		return nodes
	}
	if mac := identity.Mac(); mac != nil {
		return []identifiers.MacAddress{*mac}
	}
	return []identifiers.MacAddress{}
}

// fetchModern mirrors `_fetch_modern`: fetch the per-node counts from the
// active-client diagnostics.
func fetchModern(ctx context.Context, c *registry.Context, macs []identifiers.MacAddress) (map[identifiers.MacAddress]int, error) {
	result := map[identifiers.MacAddress]int{}
	for _, mac := range macs {
		count, err := fetchModernOne(ctx, c, mac)
		if err != nil {
			return nil, err
		}
		if count != nil {
			result[mac] = *count
		}
	}
	return result, nil
}

// truthy mirrors Python truthiness of a support value.
func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case int:
		return v != 0
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return true
}

// fetchLegacy mirrors `_fetch_legacy`: count the online clients per node,
// excluding the AI board.
func fetchLegacy(ctx context.Context, c *registry.Context, identity *device.ARDeviceIdentity, macs []identifiers.MacAddress) (map[identifiers.MacAddress]int, error) {
	raw, err := c.Read(ctx, endpoint.AREndpointFetchData,
		endpoint.HookRequest(endpoint.ARHookClientlist, endpoint.ARHookClientlistDatabase))
	if err != nil {
		return nil, err
	}
	all := clients.BuildClients(raw, identity)
	skipAiboard := truthy(identity.Support()[support.ARSupportTypeAi])

	perNode := map[identifiers.MacAddress]int{}
	for _, client := range all {
		if !client.Online || client.Connection == nil {
			continue
		}
		if skipAiboard && isAiboard(client) {
			continue
		}
		if node := client.Connection.Node; node != nil {
			perNode[*node]++
		}
	}

	// Report every requested node, filling absent ones with zero
	result := make(map[identifiers.MacAddress]int, len(macs))
	for _, mac := range macs {
		result[mac] = perNode[mac]
	}
	return result, nil
}

// fetchState mirrors `fetch_state`: fetch per-node connected clients
// counts, preferring diagnostics.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	s, ok := src.(*ARClientsCountSource)
	if !ok || s == nil {
		s = ARClientsCountSourceUniversal
	}
	identity := c.Identity
	if identity == nil {
		identity = device.NewARDeviceIdentity()
	}

	macs := targetMacs(s, identity)
	if len(macs) == 0 {
		return map[identifiers.MacAddress]int{}, nil
	}

	modern, err := fetchModern(ctx, c, macs)
	if err != nil {
		return nil, err
	}
	if len(modern) > 0 {
		return modern, nil
	}

	return fetchLegacy(ctx, c, identity, macs)
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:    "ARClientsCountSource",
		Default: func() source.DataSource { return &ARClientsCountSource{} },
		Fetch:   fetchState,
	})
}
