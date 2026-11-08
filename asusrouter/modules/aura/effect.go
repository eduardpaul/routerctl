package aura

import (
	"github.com/eduardpaul/routerctl/asusrouter/tools/color"
)

// Default color pattern, repeated across zones when no colors are stored
var defaultPattern = []color.Color{
	color.ColorFromAsus(20, 0, 128, color.ColorScaleAsusDay),
	color.ColorFromAsus(110, 0, 100, color.ColorScaleAsusDay),
	color.ColorFromAsus(128, 0, 80, color.ColorScaleAsusDay),
	color.ColorFromAsus(110, 0, 100, color.ColorScaleAsusDay),
}

// DefaultColors mirrors `default_colors`: the default color for each zone.
func DefaultColors(zones int) []color.Color {
	length := len(defaultPattern)
	out := make([]color.Color, 0, max(zones, 0))
	for index := 0; index < max(zones, 0); index++ {
		out = append(out, defaultPattern[index%length])
	}
	return out
}

// FitZones mirrors `fit_zones`: fit the color list to the zone count,
// padding from defaults.
func FitZones(colors []color.Color, zones int) []color.Color {
	if zones <= 0 {
		return append([]color.Color{}, colors...)
	}

	fitted := append([]color.Color{}, colors[:min(len(colors), zones)]...)
	if len(fitted) < zones {
		fitted = append(fitted, DefaultColors(zones)[len(fitted):]...)
	}

	return fitted
}

// ApplyColor mirrors `apply_color`: apply new color(s) to the zones,
// preserving each zone's brightness.
//
// c is Python `Color | list[Color] | None`: a color.Color (or a non-nil
// *color.Color), a []color.Color, or nil. zone may be nil.
func ApplyColor(colors []color.Color, c any, zone *int) []color.Color {
	switch v := c.(type) {
	case nil:
		return colors
	case *color.Color:
		if v == nil {
			return colors
		}
		return ApplyColor(colors, *v, zone)
	case color.Color:
		result := append([]color.Color{}, colors...)
		// A single color goes to one zone or, without a zone, to all of them
		if zone != nil && 0 <= *zone && *zone < len(result) {
			result[*zone] = v.WithBrightness(result[*zone].Brightness)
			return result
		}
		for index := range result {
			result[index] = v.WithBrightness(result[index].Brightness)
		}
		return result
	case []color.Color:
		result := append([]color.Color{}, colors...)
		// A list of colors is applied cyclically across the zones
		if len(v) > 0 {
			for index := range result {
				source := v[index%len(v)]
				result[index] = source.WithBrightness(result[index].Brightness)
			}
		}
		return result
	}
	return colors
}

// ApplyBrightness mirrors `apply_brightness`: apply a brightness to one
// zone or to all of them. brightness and zone may be nil.
func ApplyBrightness(colors []color.Color, brightness *int, zone *int) []color.Color {
	if brightness == nil {
		return colors
	}

	result := append([]color.Color{}, colors...)
	if zone != nil && 0 <= *zone && *zone < len(result) {
		result[*zone] = result[*zone].WithBrightness(*brightness)
		return result
	}

	for index := range result {
		result[index] = result[index].WithBrightness(*brightness)
	}
	return result
}
