// Package source mirrors asusrouter/modules/wifi/source.py (split out of
// package wifi to break the identity import cycle).
package source

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// Raw `bw` index -> bandwidth (Broadcom); index 4 is unused
var bwIndex = map[int]wifi.ARWiFiBandwidth{
	0: wifi.ARWiFiBandwidthAuto,
	1: wifi.ARWiFiBandwidthWidth20,
	2: wifi.ARWiFiBandwidthWidth40,
	3: wifi.ARWiFiBandwidthWidth80,
	5: wifi.ARWiFiBandwidthWidth160,
	6: wifi.ARWiFiBandwidthWidth320,
}

// readBandwidth maps the raw `bw` index to a bandwidth, or nil if unknown.
func readBandwidth(value any) any {
	index := converters.RawToInt(value)
	if index == nil {
		return nil
	}
	if bw, ok := bwIndex[*index]; ok {
		return bw
	}
	return nil
}

// A key template/hook, the field it maps to, and its converter
type fieldMap = readers.TableEntry[nvram.ARNvramIndexType, wifi.ARWiFiField]

type hookMap struct {
	hook      endpoint.ARHook
	field     wifi.ARWiFiField
	converter func(any) any
}

// Per-radio hooks whose response is a list indexed by wireless unit
var hookUnitArrays = []hookMap{
	{endpoint.ARHookWlControlChannel, wifi.ARWiFiFieldChannel, converters.AsAny(converters.RawToInt)},
}

// nvram keys stored per band, prefixed with the band value (e.g. `2g1_bw`)
var bandKeys = []fieldMap{
	{Key: nvram.ARNvramIndexTypeWlBandNmode, Field: wifi.ARWiFiFieldWirelessMode, Converter: converters.AsAny(converters.RawToInt)},
	{Key: nvram.ARNvramIndexTypeWlBandBw, Field: wifi.ARWiFiFieldBandwidth, Converter: readBandwidth},
	{Key: nvram.ARNvramIndexTypeWlBandChanspec, Field: wifi.ARWiFiFieldChannelSpec, Converter: converters.AsAny(converters.RawToStr)},
	{Key: nvram.ARNvramIndexTypeWlBandNctrlsb, Field: wifi.ARWiFiFieldSideBand, Converter: converters.AsAny(converters.RawToStr)},
	{Key: nvram.ARNvramIndexTypeWlBandBw160, Field: wifi.ARWiFiFieldEnable160mhz, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramIndexTypeWlBandBw240, Field: wifi.ARWiFiFieldEnable240mhz, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramIndexTypeWlBand11be, Field: wifi.ARWiFiFieldWifi7, Converter: converters.AsAny(converters.RawToBool)},
}

// nvram keys stored per unit, prefixed with the interface (e.g. `wl0_hwaddr`)
var unitKeys = []fieldMap{
	{Key: nvram.ARNvramIndexTypeWlRadio, Field: wifi.ARWiFiFieldState, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramIndexTypeWlCountryCode, Field: wifi.ARWiFiFieldCountry, Converter: converters.AsAny(converters.RawToStr)},
	{Key: nvram.ARNvramIndexTypeWlVersion, Field: wifi.ARWiFiFieldDriver, Converter: converters.AsAny(converters.RawToStr)},
	{Key: nvram.ARNvramIndexTypeWlHwaddr, Field: wifi.ARWiFiFieldMac, Converter: converters.AsAny(identifiers.MacAddressFromValueSafe)},
}

// appGet hooks that yield the per-unit radio arrays
var wifiHooks = func() []endpoint.ARHook {
	out := make([]endpoint.ARHook, len(hookUnitArrays))
	for i, h := range hookUnitArrays {
		out[i] = h.hook
	}
	return out
}()

// ARWiFiSource is the AsusRouter WiFi data source (router-global, all
// bands).
type ARWiFiSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARWiFiSource) SourceName() string { return "ARWiFiSource" }

// ARWiFiSourceUniversal is the universal instance - preferred.
var ARWiFiSourceUniversal = &ARWiFiSource{}

// buildRequest builds the appGet request for all WiFi bands of the device
// ("" when there is nothing to request).
func buildRequest(identity *device.ARDeviceIdentity) string {
	if identity == nil {
		return ""
	}
	wifiMap := identity.Wifi()
	if len(wifiMap) == 0 {
		return ""
	}

	items := []endpoint.ARHookItem{}
	for _, hook := range wifiHooks {
		items = append(items, hook)
	}
	for _, band := range identity.WifiBands() {
		unit := wifiMap[band]
		for _, entry := range bandKeys {
			items = append(items, nvram.NewARNvramIndexSource(entry.Key, string(band)))
		}
		for _, entry := range unitKeys {
			items = append(items, nvram.NewARNvramIndexSource(entry.Key, unit))
		}
	}

	return endpoint.HookRequest(items...)
}

// fetchState fetches the raw WiFi hooks and nvram config for all bands.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	request := buildRequest(c.Identity)
	if request == "" {
		return map[string]any{}, nil
	}

	return c.Read(ctx, endpoint.AREndpointFetchData, request)
}

// arrayAt returns the item at index of a list, or nil.
func arrayAt(value any, index int) any {
	if list, ok := value.([]any); ok && index >= 0 && index < len(list) {
		return list[index]
	}
	return nil
}

// translateBand builds the field dict for a single band/unit.
func translateBand(data map[string]any, band wifi.ARWiFiBand, unit int) map[wifi.ARWiFiField]any {
	fields := map[wifi.ARWiFiField]any{}

	for _, h := range hookUnitArrays {
		if value := converters.RawConvert(arrayAt(data[string(h.hook)], unit), h.converter); value != nil {
			fields[h.field] = value
		}
	}

	for k, v := range readers.ReadTable(data, bandKeys, func(kind nvram.ARNvramIndexType) string { return kind.Key(string(band)) }) {
		fields[k] = v
	}
	for k, v := range readers.ReadTable(data, unitKeys, func(kind nvram.ARNvramIndexType) string { return kind.Key(unit) }) {
		fields[k] = v
	}

	return fields
}

// translateState translates raw WiFi data into the per-band field dict.
func translateState(raw any, tc *registry.TranslateContext) any {
	result := map[wifi.ARWiFiBand]map[wifi.ARWiFiField]any{}
	data, ok := raw.(map[string]any)
	if !ok || tc == nil || tc.Identity == nil {
		return result
	}
	wifiMap := tc.Identity.Wifi()
	if len(wifiMap) == 0 {
		return result
	}

	for band, unit := range wifiMap {
		if fields := translateBand(data, band, unit); len(fields) > 0 {
			result[band] = fields
		}
	}

	return result
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARWiFiSource",
		Default:   func() source.DataSource { return &ARWiFiSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
