// Network data source for AsusRouter.
//
// The wireless networks (main + guest/IoT). On modern firmware they live in
// SDN; on legacy firmware they come from per-band nvram. `fetchState` picks the
// backend by the `MaxRule_SDN` support flag; `translateState` picks the parser
// by the shape of the fetched data. Both yield the same output: networks grouped
// by type -> list of network profiles.

package network

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// ARNetworkSource mirrors `ARNetworkSource`: AsusRouter network data
// source (router-global).
type ARNetworkSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARNetworkSource) SourceName() string { return "ARNetworkSource" }

// Universal instance - preferred
var ARNetworkSourceUniversal = &ARNetworkSource{}

// sdnSupported mirrors `_sdn_supported`: whether the device supports SDN
// (the `mtlancfg` backend flag).
func sdnSupported(identity *device.ARDeviceIdentity) bool {
	if identity == nil {
		return false
	}
	return sdnFlag(identity.Support())
}

// sdnFlag is the `support_value(...) is True` check of `_sdn_supported`.
func sdnFlag(flags map[support.ARSupportType]any) bool {
	value, ok := support.SupportValue(flags, support.ARSupportTypeSdn).(bool)
	return ok && value
}

// identityWifi mirrors `identity.wifi if identity is not None else {}`.
func identityWifi(identity *device.ARDeviceIdentity) map[wifi.ARWiFiBand]int {
	if identity == nil {
		return map[wifi.ARWiFiBand]int{}
	}
	return identity.Wifi()
}

// identityBands is the Python dict order of `identity.wifi`.
func identityBands(identity *device.ARDeviceIdentity) []wifi.ARWiFiBand {
	if identity == nil {
		return nil
	}
	return identity.WifiBands()
}

// fetchState mirrors `fetch_state`: fetch the networks from SDN or, on
// legacy firmware, per-band nvram.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	if sdnSupported(c.Identity) {
		return SdnFetch(ctx, c.Read)
	}

	return LegacyFetch(ctx, c.Read, identityWifi(c.Identity), identityBands(c.Identity))
}

// translateState mirrors `translate_state`: translate the fetched data
// into networks grouped by type.
func translateState(raw any, tc *registry.TranslateContext) any {
	return translate(raw, tcIdentity(tc))
}

func tcIdentity(tc *registry.TranslateContext) *device.ARDeviceIdentity {
	if tc == nil {
		return nil
	}
	return tc.Identity
}

// translate is the typed body of `translate_state`.
func translate(raw any, identity *device.ARDeviceIdentity) map[ARNetworkType][]map[ARNetworkField]any {
	return translateWifi(raw, identityWifi(identity), identityBands(identity))
}

// translateWifi is `translate` with the identity reduced to its wifi map
// (the only identity data the translation reads).
func translateWifi(raw any, wifiMap map[wifi.ARWiFiBand]int, order []wifi.ARWiFiBand) map[ARNetworkType][]map[ARNetworkField]any {
	data, ok := raw.(map[string]any)
	if !ok {
		return map[ARNetworkType][]map[ARNetworkField]any{}
	}

	if _, ok := data[string(nvram.ARNvramTypeSdnRl)]; ok {
		// `list(identity.wifi)`: the bands in the identity order
		var bands []wifi.ARWiFiBand
		for _, item := range wifiItems(wifiMap, order) {
			bands = append(bands, item.band)
		}
		return SdnTranslate(data, bands)
	}

	return LegacyTranslate(data, wifiMap, order)
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARNetworkSource",
		Default:   func() source.DataSource { return &ARNetworkSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
