package aura

import (
	"context"
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/color"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

// ARAuraAction mirrors `ARAuraAction`: change the Aura light effect,
// colors or night mode. Every field is optional (nil = Python `None`).
type ARAuraAction struct {
	action.Base

	Scheme *ARAuraScheme
	// Color is Python `Color | list[Color] | None`: a color.Color (or
	// *color.Color), a []color.Color, or nil
	Color      any
	Brightness *int
	Zone       *int
	NightMode  *bool
}

// ActionName returns the Python class name.
func (*ARAuraAction) ActionName() string { return "ARAuraAction" }

// get mirrors `current.get(field, default)`.
func get(current map[ARAuraField]any, field ARAuraField, def any) any {
	if value, ok := current[field]; ok {
		return value
	}
	return def
}

// resolveScheme mirrors `_resolve_scheme`: resolve the scheme to apply
// from the action and current state.
func resolveScheme(a *ARAuraAction, current map[ARAuraField]any) ARAuraScheme {
	var raw any
	switch {
	case a.Scheme == nil:
		raw = get(current, ARAuraFieldScheme, ARAuraSchemeStatic)
	case *a.Scheme == ARAuraSchemeOn:
		raw = get(current, ARAuraFieldSchemePrev, ARAuraSchemeStatic)
	default:
		raw = *a.Scheme
	}
	scheme := ARAuraSchemeFromValue(raw)

	if a.Scheme != nil && *a.Scheme == ARAuraSchemeOff {
		return ARAuraSchemeOff
	}
	switch scheme {
	case ARAuraSchemeUnknown, ARAuraSchemeOff, ARAuraSchemeOn:
		return ARAuraSchemeStatic
	}
	return scheme
}

// buildColorPayload mirrors `_build_color_payload`: build the ledg_rgb
// payload for a color scheme.
func buildColorPayload(a *ARAuraAction, current map[ARAuraField]any, scheme ARAuraScheme) []writers.KeyValue {
	zones, _ := current[ARAuraFieldZones].(int)
	if zones < 1 {
		// No zones to color; avoid sending an empty ledg_rgb that wipes colors
		return nil
	}

	allColors, _ := current[ARAuraFieldColors].(map[ARAuraScheme][]color.Color)
	stored := allColors[scheme]
	if len(stored) == 0 {
		stored = DefaultColors(zones)
	}
	colors := FitZones(append([]color.Color{}, stored...), zones)
	colors = ApplyColor(colors, a.Color, a.Zone)
	colors = ApplyBrightness(colors, a.Brightness, a.Zone)

	return []writers.KeyValue{
		{Key: string(nvram.ARNvramTypeAuraRgb), Value: color.DumpColors(colors, color.ColorScaleAsusDay)},
	}
}

// editingColor reports whether the action carries a color (Python
// `action.color is not None`).
func editingColor(c any) bool {
	switch v := c.(type) {
	case nil:
		return false
	case *color.Color:
		return v != nil
	case []color.Color:
		return v != nil
	}
	return true
}

// update mirrors `dict.update` on ordered pairs: replace in place, append
// new keys.
func update(payload []writers.KeyValue, extra []writers.KeyValue) []writers.KeyValue {
	for _, kv := range extra {
		replaced := false
		for i := range payload {
			if payload[i].Key == kv.Key {
				payload[i].Value = kv.Value
				replaced = true
				break
			}
		}
		if !replaced {
			payload = append(payload, kv)
		}
	}
	return payload
}

// runAction mirrors `run_action`: apply the Aura action.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARAuraAction)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	if !AuraSupported(c.Identity) {
		slog.Debug("Aura unsupported on this device; ignoring action")
		return service.ARServiceResult{Success: false}, nil
	}

	current := map[ARAuraField]any{}
	if c.FetchData != nil {
		fetched, err := c.FetchData(ctx, ARAuraSourceUniversal, nil)
		if err != nil {
			return nil, err
		}
		if data, ok := fetched.(map[ARAuraField]any); ok && data != nil {
			current = data
		}
	}

	scheme := resolveScheme(act, current)
	payload := []writers.KeyValue{{Key: string(nvram.ARNvramTypeAuraScheme), Value: int(scheme)}}

	// If night mode is supported
	if act.NightMode != nil {
		if _, ok := current[ARAuraFieldNightMode]; ok {
			payload = update(payload, []writers.KeyValue{{Key: string(nvram.ARNvramTypeAuraNightMode), Value: *act.NightMode}})
		} else {
			slog.Debug("Night mode unsupported; ignoring night_mode change")
		}
	}

	editingColors := editingColor(act.Color) || act.Brightness != nil
	if AuraColorSchemes[scheme] && editingColors {
		payload = update(payload, buildColorPayload(act, current, scheme))
	}

	request := writers.DictToRequest(
		payload, endpoint.GetEndpointRequestType(endpoint.AREndpointSetAura),
	)

	// Post raw: async_fetch returns None on failure and the (ignored) body on
	// success, while async_read would collapse a failed fetch to {} and hide it
	if c.FetchRaw != nil {
		data, err := c.FetchRaw(ctx, endpoint.AREndpointSetAura, request)
		if err != nil {
			return nil, err
		}
		return service.ARServiceResult{Success: data != nil}, nil
	}
	data, err := c.Read(ctx, endpoint.AREndpointSetAura, request)
	if err != nil {
		return nil, err
	}
	return service.ARServiceResult{Success: data != nil}, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARAuraAction",
		Run:  runAction,
	})
}
