package aimesh

import (
	"context"
	"reflect"
	"testing"

	aimeshmod "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/traffic"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

const (
	nodeMAC   = "AA:BB:CC:00:00:01"
	parentMAC = "AA:BB:CC:00:00:02"
	apFhMAC   = "AA:BB:CC:11:00:01"
	apMAC     = "AA:BB:CC:11:00:02"
	staMAC    = "AA:BB:CC:22:00:01"
)

type metricsMap = map[common.ARMetricType]any
type linkMap = map[traffic.ARTrafficLink]map[common.ARMetricType]any

func rawSample() map[string]any {
	return map[string]any{
		"data":         map[string]any{"rx": int64(1), "tx": int64(2), "avg_rx": int64(3), "avg_tx": int64(4)},
		"phy":          map[string]any{"rx": int64(5), "tx": int64(6)},
		"error_status": int64(200),
	}
}

var sampleMetrics = metricsMap{
	common.ARMetricTypeRxSpeed:    1024.0,
	common.ARMetricTypeTxSpeed:    2048.0,
	common.ARMetricTypeRxSpeedAvg: 3072.0,
	common.ARMetricTypeTxSpeedAvg: 4096.0,
	common.ARMetricTypePhyRxSpeed: float64(5 << 20),
	common.ARMetricTypePhyTxSpeed: float64(6 << 20),
}

func mac(s string) *identifiers.MacAddress { return identifiers.MacAddressFromValueSafe(s) }

// testIdentity builds an identity with a single node holding the parts.
func testIdentity(radios map[wifi.ARWiFiBand]*aimeshmod.ARAiMeshRadio, backhaul *aimeshmod.ARAiMeshBackhaul, node bool) *device.ARDeviceIdentity {
	identity := device.ARDeviceIdentityBuild(device.IdentityData{source.IDOf(nvram.ARNvramTypeMac): nodeMAC})
	if node {
		if radios == nil {
			radios = map[wifi.ARWiFiBand]*aimeshmod.ARAiMeshRadio{}
		}
		topology := aimeshmod.NewARAiMeshTopology()
		topology.Add(&aimeshmod.ARAiMeshNode{Mac: *mac(nodeMAC), Radios: radios, Backhaul: backhaul})
		identity.UpdateAimesh(topology)
	}
	return identity
}

func kv(pairs ...any) []writers.KeyValue {
	var out []writers.KeyValue
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, writers.KeyValue{Key: pairs[i].(string), Value: pairs[i+1]})
	}
	return out
}

func TestResolve(t *testing.T) {
	node := mac(nodeMAC)
	band5 := wifi.ARWiFiBandBand5g1

	tests := []struct {
		name     string
		link     traffic.ARTrafficLink
		identity *device.ARDeviceIdentity
		want     *request
	}{
		{
			"wifi_band_fronthaul", band5,
			testIdentity(map[wifi.ARWiFiBand]*aimeshmod.ARAiMeshRadio{band5: {Band: band5, MacFh: mac(apFhMAC), MacAp: mac(apMAC)}}, nil, true),
			&request{endpoint.AREndpointFetchTrafficWifi, kv("node_mac", *node, "band_mac", *mac(apFhMAC))},
		},
		{
			"wifi_band_falls_back_to_ap", band5,
			testIdentity(map[wifi.ARWiFiBand]*aimeshmod.ARAiMeshRadio{band5: {Band: band5, MacAp: mac(apMAC)}}, nil, true),
			&request{endpoint.AREndpointFetchTrafficWifi, kv("node_mac", *node, "band_mac", *mac(apMAC))},
		},
		{"wifi_missing_band", band5, testIdentity(nil, nil, true), nil},
		{"wifi_no_mac", band5, testIdentity(map[wifi.ARWiFiBand]*aimeshmod.ARAiMeshRadio{band5: {Band: band5}}, nil, true), nil},
		{"wifi_no_node", band5, testIdentity(nil, nil, false), nil},
		{
			"wired_fronthaul", traffic.ARTrafficTypeWired, testIdentity(nil, nil, true),
			&request{endpoint.AREndpointFetchTrafficEthernet, kv("node_mac", *node, "is_bh", false)},
		},
		{"backhaul_none", traffic.ARTrafficTypeBackhaul, testIdentity(nil, nil, true), nil},
		{
			"backhaul_incomplete_wireless", traffic.ARTrafficTypeBackhaul,
			testIdentity(nil, &aimeshmod.ARAiMeshBackhaul{Medium: aimeshmod.ARAiMeshMediumWireless}, true), nil,
		},
		{"wan", traffic.ARTrafficTypeWan, testIdentity(nil, nil, true), nil},
		{"usb", traffic.ARTrafficTypeUsb, testIdentity(nil, nil, true), nil},
		{"lacp", traffic.ARTrafficTypeLacp, testIdentity(nil, nil, true), nil},
	}
	for _, medium := range []aimeshmod.ARAiMeshMedium{aimeshmod.ARAiMeshMediumWireless, aimeshmod.ARAiMeshMediumMlo} {
		tests = append(tests, struct {
			name     string
			link     traffic.ARTrafficLink
			identity *device.ARDeviceIdentity
			want     *request
		}{
			"backhaul_" + string(medium), traffic.ARTrafficTypeBackhaul,
			testIdentity(nil, &aimeshmod.ARAiMeshBackhaul{Medium: medium, MacParent: mac(parentMAC), MacSta: mac(staMAC)}, true),
			&request{endpoint.AREndpointFetchTrafficBackhaul, kv("node_mac", *mac(parentMAC), "sta_mac", *mac(staMAC))},
		})
	}
	for _, medium := range []aimeshmod.ARAiMeshMedium{aimeshmod.ARAiMeshMediumWired, aimeshmod.ARAiMeshMediumPlc, aimeshmod.ARAiMeshMediumMoca} {
		tests = append(tests, struct {
			name     string
			link     traffic.ARTrafficLink
			identity *device.ARDeviceIdentity
			want     *request
		}{
			"backhaul_" + string(medium), traffic.ARTrafficTypeBackhaul,
			testIdentity(nil, &aimeshmod.ARAiMeshBackhaul{Medium: medium, MacParent: mac(parentMAC)}, true),
			&request{endpoint.AREndpointFetchTrafficEthernet, kv("node_mac", *node, "is_bh", true)},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.link, node, tt.identity); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()

	reader := func(calls *[]endpoint.AREndpoint, value any) registry.ReadFunc {
		return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
			*calls = append(*calls, ep)
			if request == "" {
				t.Error("empty request")
			}
			return value, nil
		}
	}

	t.Run("atomic_calls_endpoint", func(t *testing.T) {
		var calls []endpoint.AREndpoint
		c := &registry.Context{Read: reader(&calls, rawSample()), Identity: testIdentity(nil, nil, true)}
		got, _ := fetchState(ctx, c, NewARTrafficAiMeshSource(traffic.ARTrafficTypeWired, nodeMAC))
		want := map[traffic.ARTrafficLink]any{traffic.ARTrafficTypeWired: rawSample()}
		if !reflect.DeepEqual(got, want) || len(calls) != 1 || calls[0] != endpoint.AREndpointFetchTrafficEthernet {
			t.Errorf("got %v, calls %v", got, calls)
		}
	})

	t.Run("atomic_no_endpoint", func(t *testing.T) {
		var calls []endpoint.AREndpoint
		c := &registry.Context{Read: reader(&calls, nil), Identity: testIdentity(nil, nil, true)}
		got, _ := fetchState(ctx, c, NewARTrafficAiMeshSource(traffic.ARTrafficTypeWan, nodeMAC))
		if m := got.(map[traffic.ARTrafficLink]any); len(m) != 0 || len(calls) != 0 {
			t.Errorf("got %v, calls %v", got, calls)
		}
	})

	t.Run("no_target_no_mac", func(t *testing.T) {
		var calls []endpoint.AREndpoint
		c := &registry.Context{Read: reader(&calls, nil), Identity: device.NewARDeviceIdentity()}
		got, _ := fetchState(ctx, c, NewARTrafficAiMeshSource(traffic.ARTrafficTypeWired, nil))
		if m := got.(map[traffic.ARTrafficLink]any); len(m) != 0 || len(calls) != 0 {
			t.Errorf("got %v, calls %v", got, calls)
		}
	})

	t.Run("target_defaults_to_identity_mac", func(t *testing.T) {
		var calls []endpoint.AREndpoint
		c := &registry.Context{Read: reader(&calls, rawSample()), Identity: testIdentity(nil, nil, true)}
		_, _ = fetchState(ctx, c, NewARTrafficAiMeshSource(traffic.ARTrafficTypeWired, nil))
		if len(calls) != 1 {
			t.Errorf("calls %v", calls)
		}
	})

	t.Run("aggregate_fans_out_and_merges", func(t *testing.T) {
		identity := testIdentity(
			map[wifi.ARWiFiBand]*aimeshmod.ARAiMeshRadio{wifi.ARWiFiBandBand5g1: {Band: wifi.ARWiFiBandBand5g1, MacAp: mac(apMAC)}},
			&aimeshmod.ARAiMeshBackhaul{Medium: aimeshmod.ARAiMeshMediumWired, MacParent: mac(parentMAC)},
			true,
		)
		c := &registry.Context{
			Identity: identity,
			FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
				out := map[source.ID]any{}
				for _, src := range srcs {
					out[source.IDOf(src)] = linkMap{src.(*ARTrafficAiMeshSource).Link(): sampleMetrics}
				}
				return out, nil
			},
		}
		got, _ := fetchState(ctx, c, NewARTrafficAiMeshSource(nil, nodeMAC))
		want := map[traffic.ARTrafficLink]any{
			wifi.ARWiFiBandBand5g1:        sampleMetrics,
			traffic.ARTrafficTypeWired:    sampleMetrics,
			traffic.ARTrafficTypeBackhaul: sampleMetrics,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("aggregate_without_callback", func(t *testing.T) {
		got, _ := fetchState(ctx, &registry.Context{Identity: testIdentity(nil, nil, true)}, NewARTrafficAiMeshSource(nil, nodeMAC))
		if m := got.(map[traffic.ARTrafficLink]any); len(m) != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("aggregate_no_node", func(t *testing.T) {
		var links []traffic.ARTrafficLink
		c := &registry.Context{
			Identity: testIdentity(nil, nil, false),
			FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
				for _, src := range srcs {
					links = append(links, src.(*ARTrafficAiMeshSource).Link())
				}
				return map[source.ID]any{}, nil
			},
		}
		_, _ = fetchState(ctx, c, NewARTrafficAiMeshSource(nil, nodeMAC))
		if !reflect.DeepEqual(links, []traffic.ARTrafficLink{traffic.ARTrafficTypeWired}) {
			t.Errorf("links %v", links)
		}
	})

	for name, results := range map[string]map[source.ID]any{
		"aggregate_skips_non_dict_content": {source.IDOf(NewARTrafficAiMeshSource(traffic.ARTrafficTypeWired, nodeMAC)): nil},
		"aggregate_non_dict_results":       nil,
	} {
		t.Run(name, func(t *testing.T) {
			c := &registry.Context{
				Identity: testIdentity(nil, nil, true),
				FetchDataMulti: func(context.Context, []source.DataSource, registry.Kwargs) (map[source.ID]any, error) {
					return results, nil
				},
			}
			got, _ := fetchState(ctx, c, NewARTrafficAiMeshSource(nil, nodeMAC))
			if m := got.(map[traffic.ARTrafficLink]any); len(m) != 0 {
				t.Errorf("got %v", got)
			}
		})
	}
}

func TestTranslateState(t *testing.T) {
	for _, data := range []any{nil, "not-a-dict", 5} {
		if got := translateState(data, nil).(linkMap); len(got) != 0 {
			t.Errorf("bad input %v: %v", data, got)
		}
	}

	wired := traffic.ARTrafficTypeWired
	tests := []struct {
		name string
		data map[traffic.ARTrafficLink]any
		want linkMap
	}{
		{"translates_raw", map[traffic.ARTrafficLink]any{wired: rawSample()}, linkMap{wired: sampleMetrics}},
		{"passes_through_metrics", map[traffic.ARTrafficLink]any{wired: sampleMetrics}, linkMap{wired: sampleMetrics}},
		{"empty_raw", map[traffic.ARTrafficLink]any{wired: map[string]any{}}, linkMap{wired: {}}},
		{"empty_nested_raw", map[traffic.ARTrafficLink]any{wired: map[string]any{"data": map[string]any{}}}, linkMap{wired: {}}},
		{
			"drops_unknown_keys",
			map[traffic.ARTrafficLink]any{wired: map[string]any{"weird": int64(1), "data": map[string]any{"rx": int64(1)}}},
			linkMap{wired: {common.ARMetricTypeRxSpeed: 1024.0}},
		},
		{
			"bad_status_still_translates",
			map[traffic.ARTrafficLink]any{wired: map[string]any{"data": map[string]any{"rx": int64(1)}, "error_status": int64(500)}},
			linkMap{wired: {common.ARMetricTypeRxSpeed: 1024.0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := translateState(tt.data, nil); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFlattenDict(t *testing.T) {
	nested := map[string]any{"a": map[string]any{"b": map[string]any{"c": 1}}, "d": map[string]any{"e": 2}}
	if got := flattenDict(nested, ""); !reflect.DeepEqual(got, map[string]any{"a_b_c": 1, "d_e": 2}) {
		t.Errorf("nested: %v", got)
	}
	// nil and other non-dict inputs yield an empty dict
	for _, v := range []any{nil, "not a dict"} {
		if got := flattenDict(v, ""); len(got) != 0 {
			t.Errorf("%v: %v", v, got)
		}
	}
	// A non-dict value is kept as-is under its flattened key
	if got := flattenDict(map[string]any{"a": map[string]any{"b": 1}, "d": 2}, ""); !reflect.DeepEqual(got, map[string]any{"a_b": 1, "d": 2}) {
		t.Errorf("mixed: %v", got)
	}
}

func TestRegistered(t *testing.T) {
	entry := registry.SourceByName("ARTrafficAiMeshSource")
	if entry == nil || entry.Fetch == nil || entry.Translate == nil {
		t.Fatal("not registered")
	}
}
