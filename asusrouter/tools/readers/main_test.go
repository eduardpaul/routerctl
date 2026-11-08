package readers

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// TestMain silences the decode-error logs the failure cases emit.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}
