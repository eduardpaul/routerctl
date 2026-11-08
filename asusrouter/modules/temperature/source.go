package temperature

// Temperature data source for AsusRouter.

import (
	"context"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// The temperature data can be presented in the following JS variables:
// 1) curr_coreTmp_2_raw, curr_coreTmp_5_raw, curr_coreTmp_52_raw
// 2) curr_coreTmp_0_raw, curr_coreTmp_1_raw, curr_coreTmp_2_raw,
//    curr_coreTmp_3_raw
// 3) curr_coreTmp_wl0_raw, curr_coreTmp_wl1_raw, curr_coreTmp_wl2_raw,
//    curr_coreTmp_wl3_raw
// for 2ghz, 5ghz, 5ghz2, 6ghz respectively.
// CPU temperature is set either in curr_cpuTemp or curr_coreTmp_cpu.

// Static raw JS variable -> temperature type maps, one per reporting
// format. The first format whose marker key is present in the data wins.
var formatCore52 = map[string]ARTemperatureType{
	"curr_coreTmp_2_raw":  ARTemperatureTypeRadio2g1,
	"curr_coreTmp_5_raw":  ARTemperatureTypeRadio5g1,
	"curr_coreTmp_52_raw": ARTemperatureTypeRadio5g2,
}
var formatCoreIndex = map[string]ARTemperatureType{
	"curr_coreTmp_0_raw": ARTemperatureTypeRadio2g1,
	"curr_coreTmp_1_raw": ARTemperatureTypeRadio5g1,
	"curr_coreTmp_2_raw": ARTemperatureTypeRadio5g2,
	"curr_coreTmp_3_raw": ARTemperatureTypeRadio6g1,
}

// WiFi band -> temperature type. Used to map the wlX format dynamically
// from the device identity, where each `wl{N}` index is the band id
// reported in `identity.wifi`.
var wifiBandToTemp = map[wifi.ARWiFiBand]ARTemperatureType{
	wifi.ARWiFiBandBand2g1: ARTemperatureTypeRadio2g1,
	wifi.ARWiFiBandBand2g2: ARTemperatureTypeRadio2g2,
	wifi.ARWiFiBandBand5g1: ARTemperatureTypeRadio5g1,
	wifi.ARWiFiBandBand5g2: ARTemperatureTypeRadio5g2,
	wifi.ARWiFiBandBand6g1: ARTemperatureTypeRadio6g1,
	wifi.ARWiFiBandBand6g2: ARTemperatureTypeRadio6g2,
}

// Fallback for the wlX format when the device WiFi bands are unknown.
var formatWlStatic = func() map[string]ARTemperatureType {
	out := map[string]ARTemperatureType{}
	for unit, band := range wifi.ARWifiUnitFallback {
		out["curr_coreTmp_wl"+converters.PyStr(unit)+"_raw"] = wifiBandToTemp[band]
	}
	return out
}()

// CPU temperature JS variables in priority order.
var cpuKeys = []string{"curr_coreTmp_cpu", "curr_cpuTemp"}

// identityWifi reads `identity.wifi` (a nil identity has no bands).
func identityWifi(identity *device.ARDeviceIdentity) map[wifi.ARWiFiBand]int {
	if identity == nil {
		return nil
	}
	return identity.Wifi()
}

// wlMapping builds the wlX -> temperature type map from the device WiFi
// bands.
func wlMapping(identity *device.ARDeviceIdentity) map[string]ARTemperatureType {
	out := map[string]ARTemperatureType{}
	for band, bandID := range identityWifi(identity) {
		if temp, ok := wifiBandToTemp[band]; ok {
			out["curr_coreTmp_wl"+converters.PyStr(bandID)+"_raw"] = temp
		}
	}
	return out
}

// bandMapping selects the raw variable -> temperature type map for the
// data.
//
// The wlX format is resolved from `identity.wifi` when available, since
// the `wl{N}` indices are device-specific band ids; otherwise it falls
// back to a static map. The other formats are always static.
func bandMapping(data map[string]any, identity *device.ARDeviceIdentity) map[string]ARTemperatureType {
	if _, ok := data["curr_coreTmp_5_raw"]; ok {
		return formatCore52
	}
	if _, ok := data["curr_coreTmp_0_raw"]; ok {
		return formatCoreIndex
	}
	if _, ok := data["curr_coreTmp_wl0_raw"]; ok {
		if len(identityWifi(identity)) > 0 {
			return wlMapping(identity)
		}
		return formatWlStatic
	}
	return map[string]ARTemperatureType{}
}

// ARTemperatureSource mirrors `ARTemperatureSource`: AsusRouter
// temperature data source.
type ARTemperatureSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARTemperatureSource) SourceName() string { return "ARTemperatureSource" }

// ARTemperatureSourceUniversal is the universal instance - preferred.
var ARTemperatureSourceUniversal = &ARTemperatureSource{}

// fetchState mirrors `fetch_state`: fetch the raw temperature JS
// variables.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	response, err := c.Read(ctx, endpoint.AREndpointFetchTemperature, "")
	if err != nil {
		return nil, err
	}

	if m, ok := response.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// configReader is the config a translator reads (Python takes
// `kwargs.get("config", ARConfig)`).
type configReader interface {
	notifier
}

func configOf(tc *registry.TranslateContext) configReader {
	if tc != nil {
		if cfg, ok := tc.Kwargs["config"].(configReader); ok {
			return cfg
		}
	}
	return config.ARConfig
}

// translateState mirrors `translate_state`: translate raw JS temperature
// variables to temperature values (nil for a reading that is no number).
func translateState(raw any, tc *registry.TranslateContext) any {
	data, _ := raw.(map[string]any)
	var identity *device.ARDeviceIdentity
	if tc != nil {
		identity = tc.Identity
	}
	cfg := configOf(tc)

	values := map[ARTemperatureType]any{}

	// Band detection - the wlX format is mapped from the device identity
	for jsVar, tempType := range bandMapping(data, identity) {
		values[tempType] = data[jsVar]
	}

	// CPU temperature - first available source wins
	for _, jsVar := range cpuKeys {
		if value, ok := data[jsVar]; ok {
			values[ARTemperatureTypeCpu] = value
			break
		}
	}

	// Strip degree suffix, filter out disabled sensors, and convert to float
	temperature := map[ARTemperatureType]*float64{}
	for tempType, value := range values {
		text, ok := value.(string)
		if !ok || text == "" || strings.Contains(text, "disabled") {
			continue
		}
		temperature[tempType] = converters.RawToFloat(strings.ReplaceAll(text, "&deg;C", ""))
	}

	if optimistic, _ := cfg.Get(config.ARConfigKeyOptimisticTemperature).(bool); optimistic {
		scaledTemperature, scaled := ScaleTemperature(
			temperature, ExpectedTemperatureMin, ExpectedTemperatureMax, MaxScaleCount,
		)
		temperature = map[ARTemperatureType]*float64{}
		for k, v := range scaledTemperature {
			v := v
			temperature[k] = &v
		}
		if scaled {
			WarnTemperatureScaled(cfg, data)
		}
	}

	return temperature
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARTemperatureSource",
		Default:   func() source.DataSource { return &ARTemperatureSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
