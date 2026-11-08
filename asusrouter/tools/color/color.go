// Package color ports `asusrouter/tools/color`: ASUS Aura color tools.
package color

import (
	"math"
	"strconv"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// ColorScale is the canonical full-brightness scale for a stored color.
const ColorScale = 255

// ColorScaleAsusDay is the scale of ASUS Aura day colors (stored halved).
const ColorScaleAsusDay = 128

// ColorDelimiter separates the channels of an ASUS color string.
const ColorDelimiter = ","

const channelCount = 3

// Canonical scale squared, used to re-embed brightness before rescaling
const colorScaleSq = ColorScale * ColorScale

// clamp mirrors `_clamp`: clamp the value into the inclusive range.
func clamp(value, high, low int) int {
	return max(low, min(value, high))
}

// rescale mirrors `_rescale`: rescale the value between two scales,
// clamped to the target (Python `round`: half to even).
func rescale(value, scaleTo, scaleFrom int) int {
	if scaleFrom <= 0 {
		return 0
	}
	rounded := math.RoundToEven(float64(value*scaleTo) / float64(scaleFrom))
	return clamp(int(rounded), scaleTo, 0)
}

// Color mirrors `Color`: an RGB color with hue held at full brightness
// and brightness separate.
//
// Red/Green/Blue hold the hue at full brightness on `0..ColorScale` and
// Brightness scales it on output. This mirrors how ASUS Aura encodes a
// color, where the largest channel carries the brightness and the channel
// ratios carry the hue.
//
// Build colors with NewColor (it clamps like `__post_init__`); the Python
// default `Color()` is `NewColor(0, 0, 0, ColorScale)`.
type Color struct {
	Red        int `json:"red"`
	Green      int `json:"green"`
	Blue       int `json:"blue"`
	Brightness int `json:"brightness"`
}

// NewColor mirrors `Color(red, green, blue, brightness)`: all components
// are clamped into the valid range.
func NewColor(red, green, blue, brightness int) Color {
	return Color{
		Red:        clamp(red, ColorScale, 0),
		Green:      clamp(green, ColorScale, 0),
		Blue:       clamp(blue, ColorScale, 0),
		Brightness: clamp(brightness, ColorScale, 0),
	}
}

// ColorFromAsus mirrors `Color.from_asus`: build a color from ASUS
// channels with brightness embedded.
func ColorFromAsus(r, g, b, scale int) Color {
	channels := [channelCount]int{clamp(r, scale, 0), clamp(g, scale, 0), clamp(b, scale, 0)}
	brightnessRaw := max(channels[0], channels[1], channels[2])
	if brightnessRaw == 0 {
		return NewColor(0, 0, 0, 0)
	}

	// Hue: normalize each channel to full brightness on the canonical scale
	var hue [channelCount]int
	for i, ch := range channels {
		hue[i] = rescale(ch, ColorScale, brightnessRaw)
	}
	brightness := rescale(brightnessRaw, ColorScale, scale)

	return NewColor(hue[0], hue[1], hue[2], brightness)
}

// ToAsus mirrors `to_asus`: return ASUS channels with brightness
// re-embedded at the scale.
func (c Color) ToAsus(scale int) (int, int, int) {
	// Re-embed brightness on the canonical scale, then rescale to target
	embed := func(ch int) int { return rescale(ch*c.Brightness, scale, colorScaleSq) }
	return embed(c.Red), embed(c.Green), embed(c.Blue)
}

// WithBrightness mirrors `with_brightness`: return a copy with the
// brightness replaced.
func (c Color) WithBrightness(brightness int) Color {
	c.Brightness = clamp(brightness, ColorScale, 0)
	return c
}

// ColorBlend mirrors `Color.blend`: blend colors into their average hue
// at the peak brightness.
func ColorBlend(colors []Color) Color {
	if len(colors) == 0 {
		return NewColor(0, 0, 0, ColorScale)
	}

	count := len(colors)
	red, green, blue, brightness := 0, 0, 0, colors[0].Brightness
	for _, color := range colors {
		red += color.Red
		green += color.Green
		blue += color.Blue
		brightness = max(brightness, color.Brightness)
	}

	// Channels are never negative, so `/` matches Python `//`
	return NewColor(red/count, green/count, blue/count, brightness)
}

// ParseColors mirrors `parse_colors(raw, scale=scale)` with the default
// delimiter.
func ParseColors(raw any, scale int) []Color {
	return ParseColorsWith(raw, scale, ColorDelimiter)
}

// ParseColorsWith mirrors `parse_colors`: parse a delimited ASUS color
// string into a list of colors.
func ParseColorsWith(raw any, scale int, delimiter string) []Color {
	text := converters.RawToStr(raw)
	if text == nil {
		return []Color{}
	}

	channels := strings.Split(*text, delimiter)
	channel := func(i int) int {
		if v := converters.RawToInt(channels[i]); v != nil {
			return *v
		}
		return 0
	}
	colors := []Color{}
	for i := 0; i < len(channels)-channelCount+1; i += channelCount {
		colors = append(colors, ColorFromAsus(channel(i), channel(i+1), channel(i+2), scale))
	}
	return colors
}

// DumpColors mirrors `dump_colors(colors, scale=scale)` with the default
// delimiter.
func DumpColors(colors []Color, scale int) string {
	return DumpColorsWith(colors, scale, ColorDelimiter)
}

// DumpColorsWith mirrors `dump_colors`: serialize colors back into a
// delimited ASUS color string.
func DumpColorsWith(colors []Color, scale int, delimiter string) string {
	channels := make([]string, 0, len(colors)*channelCount)
	for _, color := range colors {
		r, g, b := color.ToAsus(scale)
		channels = append(channels, strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b))
	}

	return strings.Join(channels, delimiter)
}
