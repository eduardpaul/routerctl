package temperature

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// Ported from tests/modules/temperature/test_source.py.

// identityWithWifi builds a device identity with the given band -> unit
// map (Python sets `identity._wifi`), via the wireless bands nvram and
// the WiFi units support it is read from.
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

// resetConfig mirrors the autouse fixture.
func resetConfig(t *testing.T) {
	t.Helper()
	_ = config.ARConfig.Set(config.ARConfigKeyOptimisticTemperature, false)
	_ = config.ARConfig.Set(config.ARConfigKeyNotifiedOptimisticTemperature, false)
	t.Cleanup(func() {
		_ = config.ARConfig.Set(config.ARConfigKeyOptimisticTemperature, false)
		_ = config.ARConfig.Set(config.ARConfigKeyNotifiedOptimisticTemperature, false)
	})
}

func translate(data map[string]any, identity *device.ARDeviceIdentity) map[ARTemperatureType]float64 {
	got := translateState(data, &registry.TranslateContext{Identity: identity}).(map[ARTemperatureType]*float64)
	out := map[ARTemperatureType]float64{}
	for k, v := range got {
		if v == nil {
			panic("nil temperature for " + string(k))
		}
		out[k] = *v
	}
	return out
}

func TestARTemperatureSource(t *testing.T) {
	if (&ARTemperatureSource{}).SourceName() != "ARTemperatureSource" ||
		source.IDOf(&ARTemperatureSource{}) != source.IDOf(ARTemperatureSourceUniversal) {
		t.Error("source identity")
	}
	if registry.SourceByName("ARTemperatureSource") == nil {
		t.Error("not registered")
	}
}

func TestGetState(t *testing.T) {
	t.Run("returns_response_dict", func(t *testing.T) {
		expected := map[string]any{"curr_cpuTemp": "45"}
		calls := 0
		c := &registry.Context{Read: func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
			calls++
			if ep != endpoint.AREndpointFetchTemperature || request != "" {
				t.Errorf("called with %q %q", ep, request)
			}
			return expected, nil
		}, Identity: device.NewARDeviceIdentity()}
		result, err := fetchState(context.Background(), c, ARTemperatureSourceUniversal)
		if err != nil || calls != 1 || !reflect.DeepEqual(result, expected) {
			t.Errorf("got %v, %v after %d calls", result, err, calls)
		}
	})

	for name, response := range map[string]any{"none": nil, "str": "string", "int": 42, "list": []any{}} {
		t.Run("non_dict_returns_empty_"+name, func(t *testing.T) {
			c := &registry.Context{Read: func(context.Context, endpoint.AREndpoint, string) (any, error) {
				return response, nil
			}}
			result, _ := fetchState(context.Background(), c, ARTemperatureSourceUniversal)
			if m, ok := result.(map[string]any); !ok || len(m) != 0 {
				t.Errorf("got %v", result)
			}
		})
	}
}

func TestTranslateState(t *testing.T) {
	cases := []struct {
		name string
		data map[string]any
		want map[ARTemperatureType]float64
	}{
		{"format_2_5_52",
			map[string]any{"curr_coreTmp_2_raw": "42", "curr_coreTmp_5_raw": "43", "curr_coreTmp_52_raw": "44", "curr_coreTmp_cpu": "45"},
			map[ARTemperatureType]float64{ARTemperatureTypeRadio2g1: 42, ARTemperatureTypeRadio5g1: 43, ARTemperatureTypeRadio5g2: 44, ARTemperatureTypeCpu: 45}},
		{"format_0_1_2_3",
			map[string]any{"curr_coreTmp_0_raw": "40", "curr_coreTmp_1_raw": "41", "curr_coreTmp_2_raw": "42", "curr_coreTmp_3_raw": "43", "curr_coreTmp_cpu": "50"},
			map[ARTemperatureType]float64{ARTemperatureTypeRadio2g1: 40, ARTemperatureTypeRadio5g1: 41, ARTemperatureTypeRadio5g2: 42, ARTemperatureTypeRadio6g1: 43, ARTemperatureTypeCpu: 50}},
		{"format_wl",
			map[string]any{"curr_coreTmp_wl0_raw": "38", "curr_coreTmp_wl1_raw": "39", "curr_coreTmp_wl2_raw": "40", "curr_coreTmp_wl3_raw": "41", "curr_cpuTemp": "55"},
			map[ARTemperatureType]float64{ARTemperatureTypeRadio2g1: 38, ARTemperatureTypeRadio5g1: 39, ARTemperatureTypeRadio5g2: 40, ARTemperatureTypeRadio6g1: 41, ARTemperatureTypeCpu: 55}},
		{"disabled_filtered",
			map[string]any{"curr_coreTmp_wl0_raw": "disabled", "curr_coreTmp_wl1_raw": "43", "curr_cpuTemp": "46"},
			map[ARTemperatureType]float64{ARTemperatureTypeRadio5g1: 43, ARTemperatureTypeCpu: 46}},
		{"deg_suffix_stripped", map[string]any{"curr_cpuTemp": "55&deg;C"}, map[ARTemperatureType]float64{ARTemperatureTypeCpu: 55}},
		{"cpu_priority", map[string]any{"curr_coreTmp_cpu": "60", "curr_cpuTemp": "55"}, map[ARTemperatureType]float64{ARTemperatureTypeCpu: 60}},
		{"no_data", map[string]any{}, map[ARTemperatureType]float64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetConfig(t)
			if got := translate(tc.data, device.NewARDeviceIdentity()); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v", got)
			}
		})
	}

	identityCases := []struct {
		name  string
		bands []wifi.ARWiFiBand
		units []int
		data  map[string]any
		want  map[ARTemperatureType]float64
	}{
		{"tri_band_2_5_6",
			[]wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1, wifi.ARWiFiBandBand5g1, wifi.ARWiFiBandBand6g1}, []int{0, 1, 2},
			map[string]any{"curr_coreTmp_wl0_raw": "40", "curr_coreTmp_wl1_raw": "41", "curr_coreTmp_wl2_raw": "42"},
			map[ARTemperatureType]float64{ARTemperatureTypeRadio2g1: 40, ARTemperatureTypeRadio5g1: 41, ARTemperatureTypeRadio6g1: 42}},
		{"quad_band",
			[]wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1, wifi.ARWiFiBandBand5g1, wifi.ARWiFiBandBand5g2, wifi.ARWiFiBandBand6g1}, []int{0, 1, 2, 3},
			map[string]any{"curr_coreTmp_wl0_raw": "38", "curr_coreTmp_wl1_raw": "39", "curr_coreTmp_wl2_raw": "40", "curr_coreTmp_wl3_raw": "41"},
			map[ARTemperatureType]float64{ARTemperatureTypeRadio2g1: 38, ARTemperatureTypeRadio5g1: 39, ARTemperatureTypeRadio5g2: 40, ARTemperatureTypeRadio6g1: 41}},
	}
	for _, tc := range identityCases {
		t.Run("wl_format_uses_identity_bands_"+tc.name, func(t *testing.T) {
			resetConfig(t)
			if got := translate(tc.data, identityWithWifi(tc.bands, tc.units)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v", got)
			}
		})
	}

	scaling := []struct {
		name       string
		optimistic bool
		data       map[string]any
		want       map[ARTemperatureType]float64
	}{
		{"scaled", true, map[string]any{"curr_cpuTemp": "0.04"}, map[ARTemperatureType]float64{ARTemperatureTypeCpu: 40}},
		{"not_scaled_disabled", false, map[string]any{"curr_cpuTemp": "0.04"}, map[ARTemperatureType]float64{ARTemperatureTypeCpu: 0.04}},
		{"in_range_no_warn", true, map[string]any{"curr_cpuTemp": "45"}, map[ARTemperatureType]float64{ARTemperatureTypeCpu: 45}},
	}
	for _, tc := range scaling {
		t.Run("optimistic_scaling_"+tc.name, func(t *testing.T) {
			resetConfig(t)
			_ = config.ARConfig.Set(config.ARConfigKeyOptimisticTemperature, tc.optimistic)
			if got := translate(tc.data, device.NewARDeviceIdentity()); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v", got)
			}
		})
	}

	// Go-only: a value that is no number stays as None, like Python
	t.Run("unparseable_value_is_none", func(t *testing.T) {
		resetConfig(t)
		got := translateState(map[string]any{"curr_cpuTemp": "abc"}, nil).(map[ARTemperatureType]*float64)
		if v, ok := got[ARTemperatureTypeCpu]; !ok || v != nil {
			t.Errorf("got %v", got)
		}
	})
}
