package aura

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/color"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Scheme codes that carry stored colors in nvram (Python `range(8)`)
const schemeCodes = 8

// Per-scheme day color sources (`ledg_rgb<code>`)
var colorSources = func() []*nvram.ARNvramIndexSource {
	out := make([]*nvram.ARNvramIndexSource, 0, schemeCodes)
	for code := 0; code < schemeCodes; code++ {
		out = append(out, nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeAuraRgb, code))
	}
	return out
}()

// Full NVRAM request for the Aura state, built once
var auraRequest = func() []nvram.ARNvramItem {
	out := []nvram.ARNvramItem{
		nvram.ARNvramTypeAura,
		nvram.ARNvramTypeAuraScheme,
		nvram.ARNvramTypeAuraSchemePrev,
		nvram.ARNvramTypeAuraCount,
		nvram.ARNvramTypeAuraNightMode,
	}
	for _, src := range colorSources {
		out = append(out, src)
	}
	return out
}()

// ARAuraSource mirrors `ARAuraSource`: AsusRouter Aura light effect data
// source.
type ARAuraSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARAuraSource) SourceName() string { return "ARAuraSource" }

// ARAuraSourceUniversal is the universal instance - preferred.
var ARAuraSourceUniversal = &ARAuraSource{}

// fetchState mirrors `fetch_state`: fetch the Aura configuration through
// the NVRAM module.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	if !AuraSupported(c.Identity) {
		return map[source.ID]any{}, nil
	}

	return nvramsource.AsyncFetchValues(ctx, c, auraRequest, nil)
}

// parseSchemeColors mirrors `_parse_scheme_colors`: parse per-scheme
// colors from the indexed color sources.
func parseSchemeColors(data map[source.ID]any, scale int) map[ARAuraScheme][]color.Color {
	colors := map[ARAuraScheme][]color.Color{}
	for _, src := range colorSources {
		parsed := color.ParseColors(data[source.IDOf(src)], scale)
		if len(parsed) > 0 {
			colors[ARAuraScheme(src.Index.(int))] = parsed
		}
	}

	return colors
}

// translateState mirrors `translate_state`: translate raw Aura nvram into
// a structured dict. The data is keyed by NVRAM item ID (the nvram fetch
// result).
func translateState(raw any, tc *registry.TranslateContext) any {
	data, ok := raw.(map[source.ID]any)
	if !ok || len(data) == 0 {
		return map[ARAuraField]any{}
	}
	var identity = identityOf(tc)

	scheme := ARAuraSchemeFromValue(data[source.IDOf(nvram.ARNvramTypeAuraScheme)])
	schemePrev := ARAuraSchemeFromValue(data[source.IDOf(nvram.ARNvramTypeAuraSchemePrev)])

	stateRaw := converters.RawToBool(data[source.IDOf(nvram.ARNvramTypeAura)])
	state := stateRaw != nil && *stateRaw
	if scheme == ARAuraSchemeOff {
		state = false
	}

	dayColors := parseSchemeColors(data, color.ColorScaleAsusDay)

	zones := 0
	if count := converters.RawToInt(data[source.IDOf(nvram.ARNvramTypeAuraCount)]); count != nil {
		zones = *count
	}
	if zones == 0 {
		zones = len(dayColors[ARAuraSchemeStatic])
	}

	result := map[ARAuraField]any{
		ARAuraFieldState:      state,
		ARAuraFieldScheme:     scheme,
		ARAuraFieldSchemePrev: schemePrev,
		ARAuraFieldZones:      zones,
		ARAuraFieldColors:     dayColors,
	}

	if NightModeSupported(identity) {
		nightMode := converters.RawToBool(data[source.IDOf(nvram.ARNvramTypeAuraNightMode)])
		result[ARAuraFieldNightMode] = nightMode != nil && *nightMode
	}

	// Active scheme summary
	var active []color.Color
	if AuraColorSchemes[scheme] {
		active = dayColors[scheme]
	}
	if len(active) > 0 {
		result[ARAuraFieldColor] = color.ColorBlend(active)
		brightness := active[0].Brightness
		for _, c := range active[1:] {
			brightness = max(brightness, c.Brightness)
		}
		result[ARAuraFieldBrightness] = brightness
	}

	return result
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARAuraSource",
		Default:   func() source.DataSource { return &ARAuraSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}

// identityOf reads the `identity` keyword (nil without a context).
func identityOf(tc *registry.TranslateContext) *device.ARDeviceIdentity {
	if tc == nil {
		return nil
	}
	return tc.Identity
}
