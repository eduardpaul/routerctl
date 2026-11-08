// Package trafficinterface mirrors asusrouter/modules/traffic/interface/.
//
// `interface` is a Go keyword, so the package is named `trafficinterface`
// (the directory keeps the Python name).
package trafficinterface

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/traffic"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

// now is the sample clock (injectable for tests)
var now = time.Now

// Legacy appGet hook that yields the same `netdev` payload
var appgetNetdevRequest = endpoint.HookRequest(endpoint.ARHookCall{Hook: endpoint.ARHookNetdev, Args: "appobj"})

// Bits per byte, for byte-counter delta -> bit rate
const bitsPerByte = 8

// Shortest interval that yields a meaningful speed; below it, report 0
const minDeltaTime = 1.0

// Raw interface name -> fixed traffic type
var staticMap = map[string]traffic.ARTrafficType{
	"INTERNET":  traffic.ARTrafficTypeWan,
	"INTERNET1": traffic.ARTrafficTypeUsb,
	"WIRED":     traffic.ARTrafficTypeWired,
	"BRIDGE":    traffic.ARTrafficTypeBridge,
	"LACP1":     traffic.ARTrafficTypeLacp1,
	"LACP2":     traffic.ARTrafficTypeLacp2,
}

const wirelessPrefix = "WIRELESS"

// Interfaces summed into the cumulative LACP link
var lacpParts = []string{"LACP1", "LACP2"}

// Counters are byte counters per interface: {iface: {"rx": n, "tx": n}}
type Counters = map[string]map[string]int

// ARTrafficInterfaceSource mirrors `ARTrafficInterfaceSource`: the
// interface traffic source for the connected router.
//
// A single fetch returns every available interface, so there is no
// per-link variant; the previous sample is stashed here for speeds.
type ARTrafficInterfaceSource struct {
	traffic.ARTrafficSource

	mu     sync.Mutex
	prev   Counters
	prevTS *time.Time
}

// NewARTrafficInterfaceSource initializes the interface traffic source.
func NewARTrafficInterfaceSource(target any) *ARTrafficInterfaceSource {
	s := &ARTrafficInterfaceSource{}
	s.Init(nil, target)
	return s
}

// SourceName returns the Python class name.
func (*ARTrafficInterfaceSource) SourceName() string { return "ARTrafficInterfaceSource" }

// Stash stores the latest sample and returns the previous one.
func (s *ARTrafficInterfaceSource) Stash(counters Counters, timestamp time.Time) (Counters, *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, prevTS := s.prev, s.prevTS
	s.prev, s.prevTS = counters, &timestamp
	return prev, prevTS
}

// toCounters coerces a parsed netdev payload to Counters (nil if it is
// not one).
func toCounters(value any) Counters {
	switch v := value.(type) {
	case map[string]map[string]int:
		return v
	case map[string]any:
		out := Counters{}
		for iface, raw := range v {
			bucket, ok := raw.(map[string]any)
			if !ok {
				if typed, ok := raw.(map[string]int); ok {
					out[iface] = typed
				}
				continue
			}
			counters := map[string]int{}
			for kind, n := range bucket {
				if i := converters.RawToInt(n); i != nil {
					counters[kind] = *i
				}
			}
			out[iface] = counters
		}
		return out
	}
	return nil
}

func fetch(ctx context.Context, c *registry.Context) (Counters, error) {
	// Fetch counters from `update.cgi`, falling back to appGet.
	//
	// The fallback reads the raw appGet content (not the shared JSON
	// reader, which would collapse the dual-WAN duplicates) and runs it
	// through the same netdev parser, so duplicates are summed in both
	// paths.
	request := writers.DictToRequest(
		[]writers.KeyValue{{Key: "output", Value: "netdev"}},
		endpoint.GetEndpointRequestType(endpoint.AREndpointFetchUpdate),
	)
	modern, err := c.Read(ctx, endpoint.AREndpointFetchUpdate, request)
	if err != nil {
		if !errors.Is(err, arerror.Err404) {
			return nil, err
		}
		modern = nil
	}
	if counters := toCounters(modern); len(counters) > 0 {
		return counters, nil
	}

	if c.FetchRaw == nil {
		return Counters{}, nil
	}
	legacy, err := c.FetchRaw(ctx, endpoint.AREndpointFetchData, appgetNetdevRequest)
	if err != nil {
		return nil, err
	}
	if legacy == nil {
		return Counters{}, nil
	}
	return readers.ReadNetdev(*legacy), nil
}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch interface counters and stash them for the next speed delta
	s := src.(*ARTrafficInterfaceSource)

	nowCounters, err := fetch(ctx, c)
	if err != nil {
		return nil, err
	}
	nowTS := now().UTC()
	prev, prevTS := s.Stash(nowCounters, nowTS)
	var deltaTime *float64
	if prevTS != nil {
		d := nowTS.Sub(*prevTS).Seconds()
		deltaTime = &d
	}
	return map[string]any{"now": nowCounters, "prev": prev, "delta_time": deltaTime}, nil
}

func mapIface(iface string, wifiByUnit map[int]wifi.ARWiFiBand) traffic.ARTrafficLink {
	// Resolve a raw interface name to its traffic link, or nil
	if fixed, ok := staticMap[iface]; ok {
		return fixed
	}
	if strings.HasPrefix(iface, wirelessPrefix) {
		unit := converters.RawToInt(iface[len(wirelessPrefix):])
		if unit == nil {
			return nil
		}
		if band, ok := wifiByUnit[*unit]; ok {
			return band
		}
	}
	return nil
}

func speed(value int, prev map[string]int, kind string, deltaTime *float64) float64 {
	// Bit rate from the counter delta, or 0 when not measurable.
	//
	// Returns 0 without a previous sample, for intervals shorter than a
	// second (too short to be meaningful) and for negative deltas (the
	// counter wrapped or reset on the device).
	if prev == nil || deltaTime == nil || *deltaTime < minDeltaTime {
		return 0.0
	}
	previous, ok := prev[kind]
	if !ok {
		return 0.0
	}
	delta := value - previous
	if delta < 0 {
		return 0.0
	}
	return bitsPerByte * float64(delta) / *deltaTime
}

func metrics(nowCounters map[string]int, prev map[string]int, deltaTime *float64) map[common.ARMetricType]any {
	// Build counter and speed metrics for one interface
	kinds := []struct {
		kind    string
		counter common.ARMetricType
		speed   common.ARMetricType
	}{
		{"rx", common.ARMetricTypeRx, common.ARMetricTypeRxSpeed},
		{"tx", common.ARMetricTypeTx, common.ARMetricTypeTxSpeed},
	}

	result := map[common.ARMetricType]any{}
	for _, k := range kinds {
		value, ok := nowCounters[k.kind]
		if !ok {
			continue
		}
		result[k.counter] = value
		result[k.speed] = speed(value, prev, k.kind, deltaTime)
	}
	return result
}

func sumCounters(counters Counters, parts []string) map[string]int {
	// Sum byte counters across the given interfaces, or nil if absent
	var present []map[string]int
	for _, p := range parts {
		if c, ok := counters[p]; ok {
			present = append(present, c)
		}
	}
	if len(present) == 0 {
		return nil
	}
	out := map[string]int{}
	for _, kind := range []string{"rx", "tx"} {
		found := false
		sum := 0
		for _, c := range present {
			if v, ok := c[kind]; ok {
				found = true
				sum += v
			}
		}
		if found {
			out[kind] = sum
		}
	}
	return out
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate counters into the flat per-link traffic metrics
	data, ok := raw.(map[string]any)
	if !ok {
		return map[traffic.ARTrafficLink]map[common.ARMetricType]any{}
	}

	nowCounters := toCounters(data["now"])
	prev := toCounters(data["prev"])
	var deltaTime *float64
	switch d := data["delta_time"].(type) {
	case *float64:
		deltaTime = d
	case float64:
		deltaTime = &d
	}
	var wifiByUnit map[int]wifi.ARWiFiBand
	if tc != nil && tc.Identity != nil {
		wifiByUnit = tc.Identity.WifiByUnit()
	}

	result := map[traffic.ARTrafficLink]map[common.ARMetricType]any{}
	for iface, counters := range nowCounters {
		link := mapIface(iface, wifiByUnit)
		if link == nil {
			continue
		}
		var prevIface map[string]int
		if len(prev) > 0 {
			prevIface = prev[iface]
		}
		result[link] = metrics(counters, prevIface, deltaTime)
	}

	// Cumulative LACP from its physical members
	if lacpNow := sumCounters(nowCounters, lacpParts); lacpNow != nil {
		result[traffic.ARTrafficTypeLacp] = metrics(lacpNow, sumCounters(prev, lacpParts), deltaTime)
	}

	return result
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARTrafficInterfaceSource",
		Default:   func() source.DataSource { return NewARTrafficInterfaceSource(nil) },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
