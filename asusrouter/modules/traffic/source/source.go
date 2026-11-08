// Package source mirrors asusrouter/modules/traffic/source.py: the
// `ARTrafficSource` dispatcher.
//
// It is a subpackage (not part of `traffic`) because it imports the
// `traffic/aimesh` and `traffic/interface` subpackages, which import
// `traffic` themselves.
package source

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	arsource "github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/traffic"
	trafficaimesh "github.com/eduardpaul/routerctl/asusrouter/modules/traffic/aimesh"
	trafficinterface "github.com/eduardpaul/routerctl/asusrouter/modules/traffic/interface"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// Links served only by the interface submodule (no AiMesh fetch for them)
var interfaceOnly = map[traffic.ARTrafficType]bool{
	traffic.ARTrafficTypeWan:    true,
	traffic.ARTrafficTypeUsb:    true,
	traffic.ARTrafficTypeBridge: true,
	traffic.ARTrafficTypeLacp:   true,
	traffic.ARTrafficTypeLacp1:  true,
	traffic.ARTrafficTypeLacp2:  true,
}

// Metrics is the per-link traffic content of every traffic source.
type Metrics = map[traffic.ARTrafficLink]map[common.ARMetricType]any

func content(value any) Metrics {
	// Coerce a fetched source content to a dict
	if m, ok := value.(Metrics); ok {
		return m
	}
	return Metrics{}
}

func isInterfaceOnly(link traffic.ARTrafficLink) bool {
	t, ok := link.(traffic.ARTrafficType)
	return ok && interfaceOnly[t]
}

// FetchState mirrors `fetch_state`: dispatch and merge AiMesh and
// interface traffic for the source.
func FetchState(ctx context.Context, c *registry.Context, src arsource.DataSource) (any, error) {
	if c.FetchDataMulti == nil {
		return Metrics{}, nil
	}
	s := src.(*traffic.ARTrafficSource)

	target := s.Target()
	link := s.Link()
	isSelf := target == nil
	if !isSelf && c.Identity != nil {
		if mac := c.Identity.Mac(); mac != nil && *mac == *target {
			isSelf = true
		}
	}

	// AiMesh serves bands / WIRED / BACKHAUL; interface serves the rest.
	// Both go into one pipeline request, so they are fetched concurrently
	var request []arsource.DataSource
	var aimeshSource, interfaceSource arsource.DataSource
	if !isInterfaceOnly(link) {
		aimeshSource = trafficaimesh.NewARTrafficAiMeshSource(link, target)
		request = append(request, aimeshSource)
	}
	// Interface (counters + speeds) only for the connected router, and not
	// for the backhaul link which it cannot report
	if isSelf && link != traffic.ARTrafficLink(traffic.ARTrafficTypeBackhaul) {
		interfaceSource = trafficinterface.NewARTrafficInterfaceSource(target)
		request = append(request, interfaceSource)
	}

	if len(request) == 0 {
		return Metrics{}, nil
	}

	results, err := c.FetchDataMulti(ctx, request, nil)
	if err != nil {
		return nil, err
	}

	aimesh, iface := Metrics{}, Metrics{}
	if aimeshSource != nil {
		aimesh = content(results[arsource.IDOf(aimeshSource)])
	}
	if interfaceSource != nil {
		iface = content(results[arsource.IDOf(interfaceSource)])
	}
	if link != nil {
		if value, ok := iface[link]; ok {
			iface = Metrics{link: value}
		} else {
			iface = Metrics{}
		}
	}

	// Interface is the base (byte counters); AiMesh overlays it and wins on
	// shared keys (its measured speeds are more precise than netdev deltas)
	merged := Metrics{}
	for key, value := range iface {
		merged[key] = copyMetrics(value)
	}
	for key, value := range aimesh {
		combined := merged[key]
		if combined == nil {
			combined = map[common.ARMetricType]any{}
		}
		for k, v := range value {
			combined[k] = v
		}
		merged[key] = combined
	}
	return merged, nil
}

func copyMetrics(m map[common.ARMetricType]any) map[common.ARMetricType]any {
	out := make(map[common.ARMetricType]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:    "ARTrafficSource",
		Default: func() arsource.DataSource { return traffic.NewARTrafficSource(nil, nil) },
		Fetch:   FetchState,
	})
}
