package ports

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// resetWarned clears the unknown-port warning cache (Python autouse fixture).
func resetWarned() {
	unknownPortsLock.Lock()
	defer unknownPortsLock.Unlock()
	reportedUnknownPorts = map[string]struct{}{}
}

// captureLogs routes the default slog logger into a buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestWarnUnknownPort(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   map[string]int
	}{
		{"WarnsOnFirstOccurrence", []string{"ZZ9"}, map[string]int{"ZZ9": 1}},
		{"DoesNotRepeatSameValue", []string{"ZZ9", "ZZ9"}, map[string]int{"ZZ9": 1}},
		{"WarnsEachDistinctValue", []string{"ZZ9", "YY8"}, map[string]int{"ZZ9": 1, "YY8": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetWarned()
			buf := captureLogs(t)
			for _, v := range tt.values {
				WarnUnknownPort(v)
			}
			for value, count := range tt.want {
				if got := strings.Count(buf.String(), value); got != count {
					t.Errorf("%s logged %d times, want %d", value, got, count)
				}
			}
		})
	}
}
