// Package aimesh mirrors asusrouter/modules/traffic/aimesh/.
package aimesh

import (
	"context"
	"log/slog"

	aimeshmod "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/traffic"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/units"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

var readKibps = units.ReadDataRate(units.UnitOfDataRateKibibitPerSecond)
var readMibps = units.ReadDataRate(units.UnitOfDataRateMebibitPerSecond)

const okStatus = 200

// Wireless backhaul mediums (use the station endpoint)
var wirelessBackhaul = map[aimeshmod.ARAiMeshMedium]bool{
	aimeshmod.ARAiMeshMediumWireless: true,
	aimeshmod.ARAiMeshMediumMlo:      true,
}

// Raw (flattened) key -> (output metric, value reader)
var translationTable = []struct {
	key    string
	metric common.ARMetricType
	reader func(any) float64
}{
	{"data_rx", common.ARMetricTypeRxSpeed, readKibps},
	{"data_tx", common.ARMetricTypeTxSpeed, readKibps},
	{"data_avg_rx", common.ARMetricTypeRxSpeedAvg, readKibps},
	{"data_avg_tx", common.ARMetricTypeTxSpeedAvg, readKibps},
	{"phy_rx", common.ARMetricTypePhyRxSpeed, readMibps},
	{"phy_tx", common.ARMetricTypePhyTxSpeed, readMibps},
}

// ARTrafficAiMeshSource mirrors `ARTrafficAiMeshSource`: the AiMesh
// traffic source.
type ARTrafficAiMeshSource struct {
	traffic.ARTrafficSource
}

// NewARTrafficAiMeshSource initializes the AiMesh traffic source.
func NewARTrafficAiMeshSource(link, target any) *ARTrafficAiMeshSource {
	s := &ARTrafficAiMeshSource{}
	s.Init(link, target)
	return s
}

// SourceName returns the Python class name.
func (*ARTrafficAiMeshSource) SourceName() string { return "ARTrafficAiMeshSource" }

// request is an endpoint plus its ordered request arguments.
type request struct {
	endpoint endpoint.AREndpoint
	args     []writers.KeyValue
}

// identityNode looks a node up in the identity's live topology.
func identityNode(identity *device.ARDeviceIdentity, mac *identifiers.MacAddress) *aimeshmod.ARAiMeshNode {
	if identity == nil || mac == nil {
		return nil
	}
	return identity.Aimesh().Get(*mac)
}

func resolveBand(link wifi.ARWiFiBand, nodeMac *identifiers.MacAddress, identity *device.ARDeviceIdentity) *request {
	// Resolve the wifi fronthaul endpoint for a band
	node := identityNode(identity, nodeMac)
	var bandMac *identifiers.MacAddress
	if node != nil {
		if radio, ok := node.Radios[link]; ok && radio != nil {
			bandMac = radio.MacFh
			if bandMac == nil {
				bandMac = radio.MacAp
			}
		}
	}
	if bandMac == nil {
		return nil
	}
	return &request{endpoint.AREndpointFetchTrafficWifi, []writers.KeyValue{
		{Key: "node_mac", Value: *nodeMac},
		{Key: "band_mac", Value: *bandMac},
	}}
}

func resolveBackhaul(nodeMac *identifiers.MacAddress, identity *device.ARDeviceIdentity) *request {
	// Resolve the backhaul endpoint from the node's uplink medium
	node := identityNode(identity, nodeMac)
	if node == nil || node.Backhaul == nil {
		return nil
	}
	backhaul := node.Backhaul
	if wirelessBackhaul[backhaul.Medium] {
		if backhaul.MacParent == nil || backhaul.MacSta == nil {
			return nil
		}
		return &request{endpoint.AREndpointFetchTrafficBackhaul, []writers.KeyValue{
			{Key: "node_mac", Value: *backhaul.MacParent},
			{Key: "sta_mac", Value: *backhaul.MacSta},
		}}
	}
	return &request{endpoint.AREndpointFetchTrafficEthernet, []writers.KeyValue{
		{Key: "node_mac", Value: *nodeMac},
		{Key: "is_bh", Value: true},
	}}
}

func resolve(link traffic.ARTrafficLink, nodeMac *identifiers.MacAddress, identity *device.ARDeviceIdentity) *request {
	// Resolve the endpoint and request args for a link via topology
	switch l := link.(type) {
	case wifi.ARWiFiBand:
		return resolveBand(l, nodeMac, identity)
	case traffic.ARTrafficType:
		switch l {
		case traffic.ARTrafficTypeWired:
			return &request{endpoint.AREndpointFetchTrafficEthernet, []writers.KeyValue{
				{Key: "node_mac", Value: *nodeMac},
				{Key: "is_bh", Value: false},
			}}
		case traffic.ARTrafficTypeBackhaul:
			return resolveBackhaul(nodeMac, identity)
		}
	}
	// WAN / USB / LACP / UNKNOWN - not served by AiMesh
	return nil
}

func getOne(ctx context.Context, c *registry.Context, link traffic.ARTrafficLink, nodeMac *identifiers.MacAddress, identity *device.ARDeviceIdentity) (map[traffic.ARTrafficLink]any, error) {
	// Fetch the raw traffic of a single link, keyed by it
	req := resolve(link, nodeMac, identity)
	if req == nil {
		return map[traffic.ARTrafficLink]any{}, nil
	}

	body := writers.DictToRequest(req.args, endpoint.GetEndpointRequestType(req.endpoint))
	data, err := c.Read(ctx, req.endpoint, body)
	if err != nil {
		return nil, err
	}
	return map[traffic.ARTrafficLink]any{link: data}, nil
}

func getAll(ctx context.Context, c *registry.Context, nodeMac *identifiers.MacAddress, identity *device.ARDeviceIdentity) (map[traffic.ARTrafficLink]any, error) {
	// Fetch all of a node's links via the cached pipeline, merged
	if c.FetchDataMulti == nil {
		return map[traffic.ARTrafficLink]any{}, nil
	}

	node := identityNode(identity, nodeMac)
	var links []traffic.ARTrafficLink
	if node != nil {
		for _, band := range wifi.ARWiFiBandMembers {
			if _, ok := node.Radios[band]; ok {
				links = append(links, band)
			}
		}
	}
	links = append(links, traffic.ARTrafficTypeWired)
	if node != nil && node.Backhaul != nil {
		links = append(links, traffic.ARTrafficTypeBackhaul)
	}

	sources := make([]source.DataSource, len(links))
	for i, link := range links {
		sources[i] = NewARTrafficAiMeshSource(link, *nodeMac)
	}
	results, err := c.FetchDataMulti(ctx, sources, nil)
	if err != nil {
		return nil, err
	}

	// Merged in request order (Python iterates the result dict)
	merged := map[traffic.ARTrafficLink]any{}
	for _, src := range sources {
		if m, ok := results[source.IDOf(src)].(map[traffic.ARTrafficLink]map[common.ARMetricType]any); ok {
			for k, v := range m {
				merged[k] = v
			}
		}
	}
	return merged, nil
}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch AiMesh traffic for the source's link (or all links)
	s := src.(*ARTrafficAiMeshSource)

	nodeMac := s.Target()
	if nodeMac == nil && c.Identity != nil {
		nodeMac = c.Identity.Mac()
	}
	if nodeMac == nil {
		return map[traffic.ARTrafficLink]any{}, nil
	}

	if s.Link() == nil {
		return getAll(ctx, c, nodeMac, c.Identity)
	}
	return getOne(ctx, c, s.Link(), nodeMac, c.Identity)
}

func flattenDict(d any, parentKey string) map[string]any {
	// Flatten a nested dict, joining nested keys with `_`
	m, ok := d.(map[string]any)
	if !ok {
		return map[string]any{}
	}

	flat := map[string]any{}
	for key, value := range m {
		newKey := key
		if parentKey != "" {
			newKey = parentKey + "_" + key
		}
		if _, nested := value.(map[string]any); nested {
			for k, v := range flattenDict(value, newKey) {
				flat[k] = v
			}
		} else {
			flat[newKey] = value
		}
	}

	return flat
}

func toMetrics(raw any) map[common.ARMetricType]any {
	// Translate a raw traffic response into metrics
	flat := flattenDict(raw, "")
	if len(flat) == 0 {
		return map[common.ARMetricType]any{}
	}

	if status := converters.RawToInt(flat["error_status"]); status != nil && *status != okStatus {
		slog.Warn("AiMesh traffic returned status", "status", *status)
	}

	out := map[common.ARMetricType]any{}
	for _, row := range translationTable {
		if value, ok := flat[row.key]; ok {
			out[row.metric] = row.reader(value)
		}
	}
	return out
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate raw (atomic) or pass through merged (aggregate) traffic
	data, ok := raw.(map[traffic.ARTrafficLink]any)
	if !ok {
		return map[traffic.ARTrafficLink]map[common.ARMetricType]any{}
	}

	out := map[traffic.ARTrafficLink]map[common.ARMetricType]any{}
	for link, value := range data {
		if m, isMetrics := value.(map[common.ARMetricType]any); isMetrics {
			out[link] = m
		} else {
			out[link] = toMetrics(value)
		}
	}
	return out
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARTrafficAiMeshSource",
		Default:   func() source.DataSource { return NewARTrafficAiMeshSource(nil, nil) },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
