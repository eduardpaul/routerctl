package aimesh

import (
	"context"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func featureSet(fs ...ARAiMeshFeature) FeatureSet {
	out := FeatureSet{}
	for _, f := range fs {
		out[f] = struct{}{}
	}
	return out
}

func TestTranslateFeaturesDecodesBits(t *testing.T) {
	// idx 1 (led) value 5 -> bits 0,2; idx 4 value 1 -> bit 0 (usb)
	features := TranslateFeatures(map[string]any{"1": "5", "4": "1"})

	want := featureSet(ARAiMeshFeatureCentralLed, ARAiMeshFeatureLedOnOff, ARAiMeshFeatureUsb)
	if !reflect.DeepEqual(features, want) {
		t.Fatalf("got %v, want %v", features.Sorted(), want.Sorted())
	}
}

func TestTranslateFeaturesMloBits(t *testing.T) {
	// bits 20 (mlo_bh) + 21 (mlo_fh)
	features := TranslateFeatures(map[string]any{"4": strconv.Itoa((1 << 20) | (1 << 21))})

	if !features.Has(ARAiMeshFeatureMloBh) || !features.Has(ARAiMeshFeatureMloFh) {
		t.Fatalf("got %v", features.Sorted())
	}
}

func TestTranslateFeaturesEmpty(t *testing.T) {
	cases := []struct {
		name       string
		capability any
	}{
		{"not_dict", "x"},
		{"empty", map[string]any{}},
		{"unknown_index", map[string]any{"99": "1"}},
		{"blank", map[string]any{"1": ""}},
		{"non_int", map[string]any{"1": "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TranslateFeatures(c.capability); len(got) != 0 {
				t.Fatalf("got %v", got.Sorted())
			}
		})
	}
}

// warnHandler records the bit list (the first backticked part) of each
// warning, like the Python test patching `_LOGGER.warning`.
type warnHandler struct{ warned *[]string }

func (h warnHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h warnHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		parts := strings.Split(r.Message, "`")
		if len(parts) > 1 {
			*h.warned = append(*h.warned, parts[1])
		}
	}
	return nil
}
func (h warnHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h warnHandler) WithGroup(string) slog.Handler      { return h }

func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	unknownCapsLock.Lock()
	reportedUnknownBits = map[int]map[int]struct{}{}
	unknownCapsLock.Unlock()
	warned := &[]string{}
	prev := slog.Default()
	slog.SetDefault(slog.New(warnHandler{warned}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return warned
}

func TestTranslateFeaturesUnknownBitsWarnPerBit(t *testing.T) {
	warned := captureWarnings(t)

	// idx 1 bit 7; idx 2 fully mapped (no warn)
	TranslateFeatures(map[string]any{"1": "128", "2": "1"})
	// idx 1 bit 7 already reported -> no warn
	TranslateFeatures(map[string]any{"1": "128"})
	// idx 5 (lacp): bits 3,5 -> all new
	TranslateFeatures(map[string]any{"5": strconv.Itoa((1 << 3) | (1 << 5))})
	// idx 5: bits 3,7 -> only 7 is new
	TranslateFeatures(map[string]any{"5": strconv.Itoa((1 << 3) | (1 << 7))})
	// idx 5: bit 3 -> nothing new
	TranslateFeatures(map[string]any{"5": strconv.Itoa(1 << 3)})

	want := []string{"[7]", "[3, 5]", "[7]"}
	if !reflect.DeepEqual(*warned, want) {
		t.Fatalf("got %v, want %v", *warned, want)
	}
}

func TestTranslateFeaturesAcknowledgedBitsNotWarned(t *testing.T) {
	warned := captureWarnings(t)

	// idx 4 value with only acknowledged unknown bits -> no warn
	TranslateFeatures(map[string]any{"4": "62498685"})
	// idx 4 with a non-acknowledged bit 30 -> warns only that one
	TranslateFeatures(map[string]any{"4": strconv.Itoa(1 << 30)})

	want := []string{"[30]"}
	if !reflect.DeepEqual(*warned, want) {
		t.Fatalf("got %v, want %v", *warned, want)
	}
}

func TestIsFullyDecoded(t *testing.T) {
	cases := []struct {
		name       string
		key, value string
		expected   bool
	}{
		{"reboot_full", "2", "1", true},
		{"lacp_full", "5", "1", true},
		{"led_all_known", "1", "127", true},
		{"led_unknown_bit7", "1", "128", false},
		{"unknown_index", "99", "1", false},
		{"non_int", "4", "x", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsFullyDecoded(c.key, c.value); got != c.expected {
				t.Fatalf("got %v, want %v", got, c.expected)
			}
		})
	}
}
