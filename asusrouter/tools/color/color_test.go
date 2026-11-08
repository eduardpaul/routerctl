package color

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestRescaleZeroSource(t *testing.T) {
	if rescale(100, 255, 0) != 0 || rescale(100, 255, -5) != 0 {
		t.Fatal("non-zero")
	}
}

func TestRescaleHalfToEven(t *testing.T) {
	// round(0.5) == 0 and round(1.5) == 2 in Python; 2.5 clamps to 1
	if rescale(1, 1, 2) != 0 || rescale(1, 3, 2) != 2 || rescale(5, 1, 2) != 1 {
		t.Fatal("not half-to-even")
	}
}

func TestColorDefaults(t *testing.T) {
	color := ColorBlend(nil)
	if color != (Color{0, 0, 0, ColorScale}) {
		t.Fatal(color)
	}
}

func TestColorClamped(t *testing.T) {
	if NewColor(300, 0, 0, 255).Red != ColorScale || NewColor(0, -5, 0, 255).Green != 0 ||
		NewColor(0, 0, 128, 255).Blue != 128 || NewColor(0, 0, 0, 999).Brightness != ColorScale {
		t.Fatal("not clamped")
	}
}

func TestFromAsus(t *testing.T) {
	if ColorFromAsus(0, 0, 0, ColorScaleAsusDay) != NewColor(0, 0, 0, 0) {
		t.Fatal("zero")
	}
	color := ColorFromAsus(10, 0, 64, ColorScaleAsusDay)
	if color.Red != int(math.RoundToEven(10.0*ColorScale/64)) || color.Green != 0 || color.Blue != ColorScale ||
		color.Brightness != int(math.RoundToEven(64.0*ColorScale/ColorScaleAsusDay)) {
		t.Fatal(color)
	}
	// 200 clamped to 128 -> full brightness red
	color = ColorFromAsus(200, 0, 0, ColorScaleAsusDay)
	if color.Red != ColorScale || color.Brightness != ColorScale {
		t.Fatal(color)
	}
}

func TestRoundTrip(t *testing.T) {
	for _, scale := range []int{ColorScaleAsusDay, ColorScale} {
		for _, rgb := range [][3]int{{10, 0, 64}, {128, 64, 32}, {0, 0, 0}, {100, 100, 100}} {
			source := [3]int{min(rgb[0], scale), min(rgb[1], scale), min(rgb[2], scale)}
			r, g, b := ColorFromAsus(source[0], source[1], source[2], scale).ToAsus(scale)
			for i, restored := range []int{r, g, b} {
				if d := source[i] - restored; d > 1 || d < -1 {
					t.Errorf("scale %d %v: %d -> %d", scale, source, source[i], restored)
				}
			}
		}
	}
}

func TestWithBrightness(t *testing.T) {
	color := NewColor(255, 128, 0, 200)
	dimmed := color.WithBrightness(50)
	if dimmed != NewColor(255, 128, 0, 50) || color.Brightness != 200 {
		t.Fatal(dimmed, color)
	}
	if NewColor(0, 0, 0, 255).WithBrightness(999).Brightness != ColorScale {
		t.Fatal("not clamped")
	}
}

func TestBlend(t *testing.T) {
	blended := ColorBlend([]Color{NewColor(200, 0, 0, 100), NewColor(0, 200, 0, 250)})
	if blended != NewColor(100, 100, 0, 250) {
		t.Fatal(blended)
	}
}

func TestParseDump(t *testing.T) {
	for _, raw := range []any{nil, "", "   "} {
		if got := ParseColors(raw, ColorScaleAsusDay); len(got) != 0 {
			t.Errorf("ParseColors(%#v) = %v", raw, got)
		}
	}
	if got := ParseColors("10,0,64,0,0,32,5", ColorScaleAsusDay); len(got) != 2 {
		t.Fatal(got)
	}
	if got := ParseColors("x,y,z", ColorScaleAsusDay); !reflect.DeepEqual(got, []Color{NewColor(0, 0, 0, 0)}) {
		t.Fatal(got)
	}
	raw := "10,0,64,64,32,16"
	dumped := DumpColors(ParseColors(raw, ColorScaleAsusDay), ColorScaleAsusDay)
	original, restored := strings.Split(raw, ","), strings.Split(dumped, ",")
	if len(original) != len(restored) {
		t.Fatal(dumped)
	}
	for i := range original {
		was, _ := strconv.Atoi(original[i])
		now, _ := strconv.Atoi(restored[i])
		if d := was - now; d > 1 || d < -1 {
			t.Errorf("%d -> %d", was, now)
		}
	}
	if DumpColors(nil, ColorScaleAsusDay) != "" {
		t.Fatal("non-empty dump")
	}
}
