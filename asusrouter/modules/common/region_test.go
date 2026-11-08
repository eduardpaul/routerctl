package common

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func intp(v int) *int { return &v }

func checkRegion(t *testing.T, value any, region ARRegion, area *int) {
	t.Helper()
	gotRegion, gotArea := TranslateRegion(value)
	if gotRegion != region || (gotArea == nil) != (area == nil) || (gotArea != nil && *gotArea != *area) {
		t.Errorf("TranslateRegion(%v) = (%v, %v), want (%v, %v)", value, gotRegion, gotArea, region, area)
	}
}

// A known territory code resolves to region + area.
func TestTranslateRegionKnownRegion(t *testing.T) {
	checkRegion(t, "US/01", ARRegionUs, intp(1))
	checkRegion(t, "EU/02", ARRegionEu, intp(2))
}

// All ASUS territory codes resolve.
func TestTranslateRegionTerritoryCodes(t *testing.T) {
	cases := []struct {
		code   string
		region ARRegion
	}{
		{"AA", ARRegionAa},
		{"CN", ARRegionCn},
		{"KR", ARRegionKr},
		{"RU", ARRegionRu},
		{"TW", ARRegionTw},
		{"UA", ARRegionUa},
	}
	for _, c := range cases {
		checkRegion(t, c.code+"/01", c.region, intp(1))
	}
}

// A code without an area yields None area.
func TestTranslateRegionNoArea(t *testing.T) {
	checkRegion(t, "US", ARRegionUs, nil)
}

// Empty or non-string input yields UNKNOWN.
func TestTranslateRegionUnusable(t *testing.T) {
	for name, value := range map[string]any{"none": nil, "empty": "", "not_str": 5} {
		t.Run(name, func(t *testing.T) { checkRegion(t, value, ARRegionUnknown, nil) })
	}
}

// captureHandler records warning messages (Python monkeypatches
// `_LOGGER.warning`; Go swaps the default slog handler).
type captureHandler struct {
	mu       sync.Mutex
	messages []string
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.Level >= slog.LevelWarn {
		h.messages = append(h.messages, r.Message)
	}
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

// An unknown code warns exactly once per distinct value.
func TestTranslateRegionUnknownWarnsOnce(t *testing.T) {
	unknownRegionsLock.Lock()
	reportedUnknownRegions = map[string]struct{}{}
	unknownRegionsLock.Unlock()

	h := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	checkRegion(t, "ZZ/01", ARRegionUnknown, intp(1))
	TranslateRegion("ZZ/01")
	TranslateRegion("XX/01")

	if len(h.messages) != 2 ||
		!strings.Contains(h.messages[0], "`ZZ/01`") ||
		!strings.Contains(h.messages[1], "`XX/01`") {
		t.Errorf("warned = %q", h.messages)
	}
}
