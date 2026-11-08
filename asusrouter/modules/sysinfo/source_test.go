package sysinfo

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// Ported from tests/modules/sysinfo/test_source.py.

type (
	M = ARMemoryType
	C = ARWlanClientCount
)

// identityWithWifi builds a device identity with the given band -> unit
// map (Python sets `identity._wifi`), via the wireless bands nvram and the
// WiFi units support it is read from.
func identityWithWifi(bands []wifi.ARWiFiBand, units []int) *device.ARDeviceIdentity {
	names := make([]string, len(bands))
	for i, b := range bands {
		names[i] = string(b)
	}
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(nvram.ARNvramTypeWirelessBands): strings.Join(names, "<"),
		source.IDOf(support.ARSupportSourceUniversal): map[support.ARSupportType]any{
			support.ARSupportTypeWifiCapabilities: map[wifi.ARWiFiCapability]any{
				wifi.ARWiFiCapabilityUnits: units,
			},
		},
	})
}

func translate(data map[string]any, identity *device.ARDeviceIdentity) map[ARSysInfoType]any {
	return translateState(data, &registry.TranslateContext{Identity: identity}).(map[ARSysInfoType]any)
}

func TestARSysInfoSource(t *testing.T) {
	if (&ARSysInfoSource{}).SourceName() != "ARSysInfoSource" {
		t.Error("source name")
	}
	if source.IDOf(&ARSysInfoSource{}) != source.IDOf(ARSysInfoSourceUniversal) {
		t.Error("sources are not equal")
	}
	if got := source.Repr(&ARSysInfoSource{}); got != "<ARSysInfoSource>" {
		t.Errorf("repr %q", got)
	}
	if registry.SourceByName("ARSysInfoSource") == nil {
		t.Error("not registered")
	}
}

func TestGetState(t *testing.T) {
	t.Run("returns_response_dict", func(t *testing.T) {
		expected := map[string]any{"conn_stats_arr": []any{"1", "2"}}
		calls := 0
		c := &registry.Context{Read: func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
			calls++
			if ep != endpoint.AREndpointFetchSysinfo || request != "" {
				t.Errorf("called with %q %q", ep, request)
			}
			return expected, nil
		}}
		result, err := fetchState(context.Background(), c, ARSysInfoSourceUniversal)
		if err != nil || calls != 1 || !reflect.DeepEqual(result, expected) {
			t.Errorf("got %v, %v after %d calls", result, err, calls)
		}
	})

	for name, response := range map[string]any{"none": nil, "str": "string", "int": 42, "list": []any{}} {
		t.Run("non_dict_returns_empty_"+name, func(t *testing.T) {
			c := &registry.Context{Read: func(context.Context, endpoint.AREndpoint, string) (any, error) {
				return response, nil
			}}
			result, _ := fetchState(context.Background(), c, ARSysInfoSourceUniversal)
			if m, ok := result.(map[string]any); !ok || len(m) != 0 {
				t.Errorf("got %v", result)
			}
		})
	}
}

func TestTranslateState(t *testing.T) {
	t.Run("full_pre_388_7", func(t *testing.T) {
		data := map[string]any{
			"wlc_0_arr":      []any{"11", "11", "11"},
			"wlc_1_arr":      []any{"3", "3", "3"},
			"conn_stats_arr": []any{"320", "86"},
			"mem_stats_arr": []any{
				"882.34", "240.57", "0.00", "52.73", "0.00", "0.00", "85328", "7.49 / 63.00 MB",
			},
			"cpu_stats_arr": []any{"1.98", "2.04", "2.01"},
		}
		want := map[ARSysInfoType]any{
			ARSysInfoTypeWlan: map[wifi.ARWiFiBand]map[ARWlanClientCount]int{
				wifi.ARWiFiBandBand2g1: {ARWlanClientCountAssociated: 11, ARWlanClientCountAuthorized: 11, ARWlanClientCountAuthenticated: 11},
				wifi.ARWiFiBandBand5g1: {ARWlanClientCountAssociated: 3, ARWlanClientCountAuthorized: 3, ARWlanClientCountAuthenticated: 3},
			},
			ARSysInfoTypeConnections: map[common.ARMetricType]int{common.ARMetricTypeTotal: 320, common.ARMetricTypeActive: 86},
			ARSysInfoTypeMemory: map[ARMemoryType]int{
				ARMemoryTypeTotal:     882340000,
				ARMemoryTypeFree:      240570000,
				ARMemoryTypeBuffers:   0,
				ARMemoryTypeCache:     52730000,
				ARMemoryTypeSwapUsed:  0,
				ARMemoryTypeSwapTotal: 0,
				ARMemoryTypeNvram:     85328,
				ARMemoryTypeJffsFree:  55510000,
				ARMemoryTypeJffsUsed:  7490000,
				ARMemoryTypeJffsTotal: 63000000,
			},
			ARSysInfoTypeLoadAverage: map[int]float64{1: 1.98, 5: 2.04, 15: 2.01},
		}
		if got := translate(data, nil); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("full_from_388_7", func(t *testing.T) {
		data := map[string]any{
			"conn_stats_arr": []any{"320", "86"},
			"mem_stats_arr": []any{
				"882.34", "566.32", "0.00", "20.11", "0.00", "0.00", "73397", "61.11", "294.60", "552.04",
			},
			"cpu_stats_arr": []any{"2.21", "1.43", "0.64"},
		}
		want := map[ARMemoryType]int{
			ARMemoryTypeTotal:     882340000,
			ARMemoryTypeFree:      566320000,
			ARMemoryTypeBuffers:   0,
			ARMemoryTypeCache:     20110000,
			ARMemoryTypeSwapUsed:  0,
			ARMemoryTypeSwapTotal: 0,
			ARMemoryTypeNvram:     73397,
			ARMemoryTypeJffsFree:  61110000,
			ARMemoryTypeUsed:      294600000,
			ARMemoryTypeAvailable: 552040000,
		}
		if got := translate(data, nil)[ARSysInfoTypeMemory]; !reflect.DeepEqual(got, want) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("empty_data", func(t *testing.T) {
		if got := translate(map[string]any{}, nil); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("unknown_band_index", func(t *testing.T) {
		data := map[string]any{}
		for _, k := range []string{"wlc_0_arr", "wlc_1_arr", "wlc_2_arr", "wlc_3_arr"} {
			data[k] = []any{"0", "0", "0"}
		}
		data["wlc_4_arr"] = []any{"1", "2", "3"}
		wlan := translate(data, nil)[ARSysInfoTypeWlan].(map[wifi.ARWiFiBand]map[ARWlanClientCount]int)
		want := map[ARWlanClientCount]int{ARWlanClientCountAssociated: 1, ARWlanClientCountAuthorized: 2, ARWlanClientCountAuthenticated: 3}
		if !reflect.DeepEqual(wlan[wifi.ARWiFiBandUnknown], want) {
			t.Errorf("got %v", wlan)
		}
	})

	t.Run("wlan_uses_identity_band_order", func(t *testing.T) {
		identity := identityWithWifi(
			[]wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1, wifi.ARWiFiBandBand5g1, wifi.ARWiFiBandBand5g2, wifi.ARWiFiBandBand6g1},
			[]int{3, 0, 1, 2},
		)
		data := map[string]any{
			"wlc_0_arr": []any{"1", "1", "1"},
			"wlc_1_arr": []any{"2", "2", "2"},
			"wlc_2_arr": []any{"3", "3", "3"},
			"wlc_3_arr": []any{"4", "4", "4"},
		}
		wlan := translate(data, identity)[ARSysInfoTypeWlan].(map[wifi.ARWiFiBand]map[ARWlanClientCount]int)
		for band, want := range map[wifi.ARWiFiBand]int{
			wifi.ARWiFiBandBand5g1: 1, wifi.ARWiFiBandBand5g2: 2, wifi.ARWiFiBandBand6g1: 3, wifi.ARWiFiBandBand2g1: 4,
		} {
			if got := wlan[band][ARWlanClientCountAssociated]; got != want {
				t.Errorf("%s: got %d, want %d", band, got, want)
			}
		}
	})

	t.Run("wlan_empty_identity_uses_fallback", func(t *testing.T) {
		data := map[string]any{"wlc_0_arr": []any{"7", "7", "7"}}
		wlan := translate(data, device.NewARDeviceIdentity())[ARSysInfoTypeWlan]
		want := map[wifi.ARWiFiBand]map[ARWlanClientCount]int{
			wifi.ARWiFiBandBand2g1: {ARWlanClientCountAssociated: 7, ARWlanClientCountAuthorized: 7, ARWlanClientCountAuthenticated: 7},
		}
		if !reflect.DeepEqual(wlan, want) {
			t.Errorf("got %v", wlan)
		}
	})

	t.Run("truncated_wlan_and_none_value", func(t *testing.T) {
		wlan := translate(map[string]any{"wlc_0_arr": []any{"5", "x"}}, nil)[ARSysInfoTypeWlan]
		want := map[wifi.ARWiFiBand]map[ARWlanClientCount]int{wifi.ARWiFiBandBand2g1: {ARWlanClientCountAssociated: 5}}
		if !reflect.DeepEqual(wlan, want) {
			t.Errorf("got %v", wlan)
		}
	})

	t.Run("truncated_connections", func(t *testing.T) {
		got := translate(map[string]any{"conn_stats_arr": []any{"10"}}, nil)[ARSysInfoTypeConnections]
		if !reflect.DeepEqual(got, map[common.ARMetricType]int{common.ARMetricTypeTotal: 10}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("connection_none_value_skipped", func(t *testing.T) {
		got := translate(map[string]any{"conn_stats_arr": []any{"x", "5"}}, nil)[ARSysInfoTypeConnections]
		if !reflect.DeepEqual(got, map[common.ARMetricType]int{common.ARMetricTypeActive: 5}) {
			t.Errorf("got %v", got)
		}
	})

	memory := func(raw ...any) map[ARMemoryType]int {
		return translate(map[string]any{"mem_stats_arr": raw}, nil)[ARSysInfoTypeMemory].(map[ARMemoryType]int)
	}

	t.Run("memory_invalid_values_skipped", func(t *testing.T) {
		got := memory("x", "10.00", "0.00", "0.00", "0.00", "0.00", "y")
		_, total := got[ARMemoryTypeTotal]
		_, nv := got[ARMemoryTypeNvram]
		if total || nv || got[ARMemoryTypeFree] != 10000000 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("memory_truncated", func(t *testing.T) {
		got := memory("882.34", "240.57")
		if !reflect.DeepEqual(got, map[ARMemoryType]int{ARMemoryTypeTotal: 882340000, ARMemoryTypeFree: 240570000}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("jffs_new_format_invalid", func(t *testing.T) {
		got := memory("1.00", "1.00", "1.00", "1.00", "1.00", "1.00", "1.00", "nan_value")
		if _, ok := got[ARMemoryTypeJffsFree]; ok {
			t.Errorf("got %v", got)
		}
	})

	t.Run("jffs_malformed_used_total", func(t *testing.T) {
		got := memory("1.00", "1.00", "1.00", "1.00", "1.00", "1.00", "1.00", "x / y MB")
		for _, k := range []ARMemoryType{ARMemoryTypeJffsUsed, ARMemoryTypeJffsTotal, ARMemoryTypeJffsFree} {
			if _, ok := got[k]; ok {
				t.Errorf("%s present: %v", k, got)
			}
		}
	})

	t.Run("load_average_truncated_and_none", func(t *testing.T) {
		got := translate(map[string]any{"cpu_stats_arr": []any{"1.5", "bad"}}, nil)[ARSysInfoTypeLoadAverage]
		if !reflect.DeepEqual(got, map[int]float64{1: 1.5}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("accepts_nil_context", func(t *testing.T) {
		if got := translateState(map[string]any{}, nil).(map[ARSysInfoType]any); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
}
