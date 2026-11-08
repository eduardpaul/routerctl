package systemstatus

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const testMac = "12:34:56:78:9A:BC"

var testMem = map[string]any{"mem_free": "400", "mem_total": "1000", "mem_used": "600"}

func identity(mac string) *device.ARDeviceIdentity {
	data := device.IdentityData{}
	if mac != "" {
		data[source.IDOf(nvram.ARNvramTypeMac)] = mac
	}
	return device.ARDeviceIdentityBuild(data)
}

func forceConfig(t *testing.T) *config.ARConnectionConfig {
	cfg := config.NewARConnectionConfig()
	if err := cfg.Set(config.ARConnectionConfigKeyForceLegacySystemStatus, true); err != nil {
		t.Fatal(err)
	}
	return cfg
}

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

// fakeRead answers every read with fn and records the calls.
func fakeRead(calls *[]call, fn func(ep endpoint.AREndpoint) any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return fn(ep), nil
	}
}

func legacyRead(calls *[]call, cpu, mem map[string]any) registry.ReadFunc {
	return fakeRead(calls, func(endpoint.AREndpoint) any {
		return map[string]any{"cpu_usage": cpu, "memory_usage": mem}
	})
}

func mac(s string) identifiers.MacAddress { return *identifiers.MacAddressFromValueSafe(s) }

func TestSourceTarget(t *testing.T) {
	if ARSystemStatusSourceUniversal.Target() != nil {
		t.Error("universal targets the router")
	}
	m := mac(testMac)
	tests := []struct {
		name  string
		value any
		want  *identifiers.MacAddress
	}{
		{"str", testMac, &m},
		{"macaddress", m, &m},
		{"none", nil, nil},
		{"invalid", "not-a-mac", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewARSystemStatusSource(tt.value).Target(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	a, b := NewARSystemStatusSource(testMac), NewARSystemStatusSource(strings.ToLower(testMac))
	if source.IDOf(a) != source.IDOf(b) {
		t.Error("same target (any case) must be equal")
	}
	if source.IDOf(a) == source.IDOf(NewARSystemStatusSource("AA:BB:CC:DD:EE:FF")) {
		t.Error("different targets must differ")
	}
	if r := source.Repr(a); !strings.HasPrefix(r, "<ARSystemStatusSource ") || !strings.Contains(r, m.String()) {
		t.Errorf("repr %q", r)
	}
}

func TestFetchStateModern(t *testing.T) {
	ctx := context.Background()

	// Without a target the request filters on the device MAC
	var calls []call
	c := &registry.Context{
		Read:     fakeRead(&calls, func(endpoint.AREndpoint) any { return map[string]any{"contents": []any{[]any{"5", "26"}}} }),
		Identity: identity(testMac),
	}
	got, err := fetchState(ctx, c, ARSystemStatusSourceUniversal)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{testMac: []any{[]any{"5", "26"}}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
	if len(calls) != 1 || calls[0].endpoint != endpoint.AREndpointFetchDiagnosticsData ||
		!strings.Contains(calls[0].request, "filter=node_mac") || !strings.Contains(calls[0].request, "content=cpu_usage") {
		t.Errorf("calls %v", calls)
	}

	// An explicit target overrides the device MAC
	node := "AA:BB:CC:DD:EE:FF"
	got, _ = fetchState(ctx, &registry.Context{
		Read:     fakeRead(&calls, func(endpoint.AREndpoint) any { return map[string]any{"contents": []any{[]any{"1", "2"}}} }),
		Identity: identity(testMac),
	}, NewARSystemStatusSource(node))
	if want := map[string]any{node: []any{[]any{"1", "2"}}}; !reflect.DeepEqual(got, want) {
		t.Errorf("explicit target: got %v", got)
	}

	// No target and no device MAC yields an empty result
	calls = nil
	got, _ = fetchState(ctx, &registry.Context{
		Read:     fakeRead(&calls, func(endpoint.AREndpoint) any { return map[string]any{"contents": []any{[]any{"5", "26"}}} }),
		Identity: identity(""),
	}, ARSystemStatusSourceUniversal)
	if m, _ := got.(map[string]any); len(m) != 0 || len(calls) != 0 {
		t.Errorf("no mac: got %v, calls %v", got, calls)
	}

	// An empty or non-dict response yields an empty result
	for _, response := range []any{map[string]any{"contents": []any{}}, map[string]any{}, "not-a-dict", nil} {
		got, _ := fetchState(ctx, &registry.Context{
			Read:     fakeRead(&calls, func(endpoint.AREndpoint) any { return response }),
			Identity: identity(testMac),
		}, ARSystemStatusSourceUniversal)
		if m, _ := got.(map[string]any); len(m) != 0 {
			t.Errorf("response %v: got %v", response, got)
		}
	}
}

func TestBuildRequestTimestamp(t *testing.T) {
	defer func(orig func() time.Time) { now = orig }(now)
	now = func() time.Time { return time.Unix(1794043107, 0) }

	got := buildRequest(identifiers.MacAddressFromValueSafe("C8:7F:54:00:00:01"))
	want := "ts=1794043107&duration=60&point=30&db=sys_detect&content=cpu_usage%3Bmem_usage&filter=node_mac%3Etxt%3EC8%3A7F%3A54%3A00%3A00%3A01%3E0"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

type statusMap = map[identifiers.MacAddress]map[ARSystemType]map[common.ARMetricType]any

func TestTranslateState(t *testing.T) {
	tc := &registry.TranslateContext{Identity: identity(testMac)}
	m := mac(testMac)

	tests := []struct {
		name string
		data any
		want statusMap
	}{
		{"translates_usage", map[string]any{testMac: []any{[]any{"5", "26"}}}, statusMap{m: {
			ARSystemTypeCpu: {common.ARMetricTypeUsage: 5},
			ARSystemTypeRam: {common.ARMetricTypeUsage: 26},
		}}},
		{"uses_latest_row", map[string]any{testMac: []any{[]any{"1", "20"}, []any{"7", "26"}, []any{"9", "26"}}}, statusMap{m: {
			ARSystemTypeCpu: {common.ARMetricTypeUsage: 9},
			ARSystemTypeRam: {common.ARMetricTypeUsage: 26},
		}}},
		{"partial_row_keeps_present", map[string]any{testMac: []any{[]any{"7"}}}, statusMap{m: {
			ARSystemTypeCpu: {common.ARMetricTypeUsage: 7},
		}}},
		{"non_int_values_omitted", map[string]any{testMac: []any{[]any{"x", "y"}}}, statusMap{}},
		{"empty", map[string]any{}, statusMap{}},
		{"str", "not-a-dict", statusMap{}},
		{"none", nil, statusMap{}},
		{"empty_contents", map[string]any{testMac: []any{}}, statusMap{}},
		{"bad_mac", map[string]any{"bad-mac": []any{[]any{"1", "2"}}}, statusMap{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := translateState(tt.data, tc); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSourceStash(t *testing.T) {
	s := NewARSystemStatusSource(nil)
	if prev := s.StashCPU(CpuCounters{1: {1, 1}}); prev != nil {
		t.Errorf("first stash returned %v", prev)
	}
	if prev := s.StashCPU(CpuCounters{1: {2, 2}}); !reflect.DeepEqual(prev, CpuCounters{1: {1, 1}}) {
		t.Errorf("second stash returned %v", prev)
	}
}

func legacyNode(t *testing.T, raw any) map[ARSystemType]map[common.ARMetricType]any {
	t.Helper()
	result := translateState(raw, &registry.TranslateContext{Identity: identity(testMac)}).(statusMap)
	return result[mac(testMac)]
}

func TestLegacyFetchState(t *testing.T) {
	ctx := context.Background()
	id := identity(testMac)

	t.Run("force_legacy_uses_appget", func(t *testing.T) {
		var calls []call
		c := &registry.Context{
			Read:             legacyRead(&calls, map[string]any{"cpu1_total": "100", "cpu1_usage": "10"}, testMem),
			Identity:         id,
			ConnectionConfig: forceConfig(t),
		}
		result, _ := fetchState(ctx, c, NewARSystemStatusSource(nil))
		if len(calls) != 1 || calls[0].endpoint != endpoint.AREndpointFetchData || calls[0].request != LegacyRequest {
			t.Fatalf("calls %v", calls)
		}
		// First sample has no previous counters, so only ram is reported
		node := legacyNode(t, result)
		if _, ok := node[ARSystemTypeCpu]; ok {
			t.Error("cpu without a previous sample")
		}
		if node[ARSystemTypeRam][common.ARMetricTypeTotal] != 1000*kib || node[ARSystemTypeRam][common.ARMetricTypeUsage] != 60.0 {
			t.Errorf("ram %v", node[ARSystemTypeRam])
		}
	})

	t.Run("modern_empty_falls_back_to_legacy", func(t *testing.T) {
		var calls []call
		c := &registry.Context{
			Read: fakeRead(&calls, func(ep endpoint.AREndpoint) any {
				if ep == endpoint.AREndpointFetchData {
					return map[string]any{"cpu_usage": map[string]any{"cpu1_total": "100", "cpu1_usage": "10"}, "memory_usage": testMem}
				}
				return map[string]any{}
			}),
			Identity: id,
		}
		result, _ := fetchState(ctx, c, NewARSystemStatusSource(nil))
		if node := legacyNode(t, result); node[ARSystemTypeRam][common.ARMetricTypeUsed] != 600*kib {
			t.Errorf("node %v", node)
		}
	})

	t.Run("usage_computed_on_second_sample", func(t *testing.T) {
		var calls []call
		s := NewARSystemStatusSource(nil)
		cfg := forceConfig(t)
		_, _ = fetchState(ctx, &registry.Context{
			Read:             legacyRead(&calls, map[string]any{"cpu1_total": "100", "cpu1_usage": "10", "cpu2_total": "100", "cpu2_usage": "20"}, testMem),
			Identity:         id,
			ConnectionConfig: cfg,
		}, s)
		result, _ := fetchState(ctx, &registry.Context{
			Read:             legacyRead(&calls, map[string]any{"cpu1_total": "200", "cpu1_usage": "60", "cpu2_total": "200", "cpu2_usage": "40"}, testMem),
			Identity:         id,
			ConnectionConfig: cfg,
		}, s)
		node := legacyNode(t, result)
		if node[ARSystemTypeCpu][common.ARMetricTypeUsage] != 35.0 ||
			node[ARSystemTypeCore1][common.ARMetricTypeUsage] != 50.0 ||
			node[ARSystemTypeCore2][common.ARMetricTypeUsage] != 20.0 {
			t.Errorf("node %v", node)
		}
	})

	t.Run("cores_beyond_members_feed_aggregate_only", func(t *testing.T) {
		var calls []call
		s := NewARSystemStatusSource(nil)
		cfg := forceConfig(t)
		first, second := map[string]any{}, map[string]any{}
		for core := 1; core < 10; core++ {
			n := string(rune('0' + core))
			first["cpu"+n+"_total"], first["cpu"+n+"_usage"] = "100", "10"
			second["cpu"+n+"_total"], second["cpu"+n+"_usage"] = "200", "60"
		}
		_, _ = fetchState(ctx, &registry.Context{Read: legacyRead(&calls, first, testMem), Identity: id, ConnectionConfig: cfg}, s)
		result, _ := fetchState(ctx, &registry.Context{Read: legacyRead(&calls, second, testMem), Identity: id, ConnectionConfig: cfg}, s)
		node := legacyNode(t, result)
		if _, ok := node[ARSystemTypeCpu]; !ok {
			t.Error("no aggregate")
		}
		cores := 0
		for systemType := range node {
			if strings.HasPrefix(string(systemType), "core_") {
				cores++
			}
		}
		if cores != 8 {
			t.Errorf("%d cores", cores)
		}
	})

	t.Run("node_target_skips_legacy", func(t *testing.T) {
		var calls []call
		result, _ := fetchState(ctx, &registry.Context{
			Read:             legacyRead(&calls, map[string]any{"cpu1_total": "100", "cpu1_usage": "10"}, testMem),
			Identity:         id,
			ConnectionConfig: forceConfig(t),
		}, NewARSystemStatusSource("AA:BB:CC:DD:EE:FF"))
		if m, _ := result.(map[string]any); len(m) != 0 || len(calls) != 0 {
			t.Errorf("got %v, calls %v", result, calls)
		}
	})

	t.Run("non_dict_response", func(t *testing.T) {
		var calls []call
		result, _ := fetchState(ctx, &registry.Context{
			Read:             fakeRead(&calls, func(endpoint.AREndpoint) any { return "not-a-dict" }),
			Identity:         id,
			ConnectionConfig: forceConfig(t),
		}, NewARSystemStatusSource(nil))
		if m, _ := result.(map[string]any); len(m) != 0 {
			t.Errorf("got %v", result)
		}
	})

	t.Run("empty_data", func(t *testing.T) {
		var calls []call
		result, _ := fetchState(ctx, &registry.Context{
			Read:             legacyRead(&calls, map[string]any{}, map[string]any{}),
			Identity:         id,
			ConnectionConfig: forceConfig(t),
		}, NewARSystemStatusSource(nil))
		if m, _ := result.(map[string]any); len(m) != 0 {
			t.Errorf("got %v", result)
		}
	})
}
