package log

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const (
	testMAC      = "aa:bb:cc:11:22:33"
	testIP       = "192.168.1.10"
	testSSID     = "my-network"
	testPassword = "s3cr3t"
)

// setup resets config and masking around each test
func setup(t *testing.T, level security.ARSecurityLevel) {
	t.Helper()
	config.ARConfig.Reset(nil)
	resetLogMasking()
	_ = config.ARConfig.Set(config.ARConfigKeySecurityLevelLog, level)
	t.Cleanup(func() {
		config.ARConfig.Reset(nil)
		resetLogMasking()
	})
}

// captureHandler renders records as `key=value` lines without quoting
type captureHandler struct {
	buf    *bytes.Buffer
	prefix string
}

func (h captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h captureHandler) Handle(_ context.Context, r slog.Record) error {
	var parts []string
	var walk func(prefix string, a slog.Attr)
	walk = func(prefix string, a slog.Attr) {
		v := a.Value.Resolve()
		if v.Kind() == slog.KindGroup {
			for _, g := range v.Group() {
				walk(prefix+a.Key+".", g)
			}
			return
		}
		parts = append(parts, prefix+a.Key+"="+v.String())
	}
	r.Attrs(func(a slog.Attr) bool { walk(h.prefix, a); return true })
	h.buf.WriteString(strings.Join(parts, " "))
	return nil
}

func (h captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h captureHandler) WithGroup(string) slog.Handler      { return h }

// logLine logs one record through the masking handler
func logLine(args ...any) string {
	var buf bytes.Buffer
	logger := InstallLogMasking(slog.New(captureHandler{buf: &buf}))
	logger.Info("", args...)
	return buf.String()
}

func mustMac(t *testing.T) identifiers.MacAddress {
	mac, err := identifiers.MacAddressFromValue(testMAC)
	if err != nil {
		t.Fatal(err)
	}
	return mac
}

func TestMacSanitizedIsMasked(t *testing.T) {
	setup(t, security.ARSecurityLevelSanitized)
	message := logLine("mac", mustMac(t))
	if strings.Contains(message, testMAC) || strings.Contains(message, security.RedactedStr) {
		t.Fatalf("message = %q", message)
	}
	mask := mustMac(t).Mask().(identifiers.MacAddress)
	if message != "mac="+mask.String() {
		t.Fatalf("message = %q, want the masked MAC", message)
	}
}

func TestMacReasonableIsRaw(t *testing.T) {
	setup(t, security.ARSecurityLevelReasonable)
	if message := logLine("mac", mustMac(t)); message != "mac="+testMAC {
		t.Fatalf("message = %q", message)
	}
}

func TestMacDefaultIsRedacted(t *testing.T) {
	setup(t, security.ARSecurityLevelDefault)
	if message := logLine("mac", mustMac(t)); message != "mac="+security.RedactedStr {
		t.Fatalf("message = %q", message)
	}
}

func TestIPSanitizedIsMasked(t *testing.T) {
	setup(t, security.ARSecurityLevelSanitized)
	ip, _ := identifiers.IpAddressFromValue(testIP)
	message := logLine("ip", ip)
	if strings.Contains(message, testIP) || strings.Contains(message, security.RedactedStr) {
		t.Fatalf("message = %q", message)
	}
}

func TestSSIDShownByDefault(t *testing.T) {
	setup(t, security.ARSecurityLevelDefault)
	if message := logLine("ssid", identifiers.SsidFromValue(testSSID)); message != "ssid="+testSSID {
		t.Fatalf("message = %q", message)
	}
}

func TestPasswordRedacted(t *testing.T) {
	for _, level := range security.ARSecurityLevelMembers {
		setup(t, level)
		message := logLine("pw", identifiers.PasswordFromValue(testPassword))
		if message != "pw="+security.RedactedStr {
			t.Fatalf("level %s: message = %q", level.Name(), message)
		}
	}
}

func TestGroupAndWithAttrsMasked(t *testing.T) {
	setup(t, security.ARSecurityLevelDefault)
	if message := logLine(slog.Group("g", "mac", mustMac(t))); message != "g.mac="+security.RedactedStr {
		t.Fatalf("group message = %q", message)
	}
	var buf bytes.Buffer
	logger := InstallLogMasking(slog.New(slog.NewTextHandler(&buf, nil))).With("mac", mustMac(t))
	logger.Info("x")
	if strings.Contains(buf.String(), testMAC) {
		t.Fatalf("WithAttrs leaked: %q", buf.String())
	}
}

func TestSensitiveWrapper(t *testing.T) {
	setup(t, security.ARSecurityLevelSanitized)
	if message := logLine("payload", security.NewSensitive("x=1")); message != "payload="+security.RedactedStr {
		t.Fatalf("message = %q", message)
	}
	if message := logLine("payload", security.Sensitive{Value: "x=1", Level: security.ARSecurityLevelDefault}); message != "payload=x=1" {
		t.Fatalf("message = %q", message)
	}
}

func TestNonSensitiveUntouched(t *testing.T) {
	setup(t, security.ARSecurityLevelStrict)
	if message := logLine("value", 42); message != "value=42" {
		t.Fatalf("message = %q", message)
	}
}

func TestLogValueWithoutHandler(t *testing.T) {
	setup(t, security.ARSecurityLevelDefault)
	if v := mustMac(t).LogValue().String(); v != security.RedactedStr {
		t.Fatalf("LogValue = %q", v)
	}
}

func TestEffectiveLogLevel(t *testing.T) {
	newInstance := func(level security.ARSecurityLevel) *config.ARInstanceConfig {
		return config.NewARInstanceConfig(map[config.ARConfigKey]any{config.ARConfigKeySecurityLevelLog: level})
	}

	t.Run("global_only", func(t *testing.T) {
		setup(t, security.ARSecurityLevelReasonable)
		if got := EffectiveLogLevel(); got != security.ARSecurityLevelReasonable {
			t.Fatal(got)
		}
	})
	t.Run("stricter_instance_wins", func(t *testing.T) {
		setup(t, security.ARSecurityLevelReasonable)
		inst := newInstance(security.ARSecurityLevelStrict)
		RegisterLogConfig(inst)
		if got := EffectiveLogLevel(); got != security.ARSecurityLevelStrict {
			t.Fatal(got)
		}
		runtime.KeepAlive(inst)
	})
	t.Run("laxer_instance_does_not_loosen", func(t *testing.T) {
		setup(t, security.ARSecurityLevelSanitized)
		inst := newInstance(security.ARSecurityLevelUnsafe)
		RegisterLogConfig(inst)
		if got := EffectiveLogLevel(); got != security.ARSecurityLevelSanitized {
			t.Fatal(got)
		}
		runtime.KeepAlive(inst)
	})
	t.Run("min_across_multiple_instances", func(t *testing.T) {
		setup(t, security.ARSecurityLevelUnsafe)
		a, b := newInstance(security.ARSecurityLevelReasonable), newInstance(security.ARSecurityLevelSanitized)
		RegisterLogConfig(a)
		RegisterLogConfig(b)
		if got := EffectiveLogLevel(); got != security.ARSecurityLevelSanitized {
			t.Fatal(got)
		}
		runtime.KeepAlive(a)
		runtime.KeepAlive(b)
	})
	t.Run("unregister", func(t *testing.T) {
		setup(t, security.ARSecurityLevelReasonable)
		inst := newInstance(security.ARSecurityLevelStrict)
		RegisterLogConfig(inst)
		if got := EffectiveLogLevel(); got != security.ARSecurityLevelStrict {
			t.Fatal(got)
		}
		UnregisterLogConfig(inst)
		if got := EffectiveLogLevel(); got != security.ARSecurityLevelReasonable {
			t.Fatal(got)
		}
	})
	t.Run("dead_instance_drops_out", func(t *testing.T) {
		setup(t, security.ARSecurityLevelReasonable)
		func() {
			RegisterLogConfig(newInstance(security.ARSecurityLevelStrict))
		}()
		for range 5 {
			runtime.GC()
		}
		if got := EffectiveLogLevel(); got != security.ARSecurityLevelReasonable {
			t.Fatal(got)
		}
	})
}

func TestRenderForLog(t *testing.T) {
	setup(t, security.ARSecurityLevelSanitized)
	mac := mustMac(t)
	result, ok := RenderForLog(mac).(identifiers.MacAddress)
	if !ok || result != mac.Mask() || result == mac {
		t.Fatalf("masked = %v", result)
	}
	_ = config.ARConfig.Set(config.ARConfigKeySecurityLevelLog, security.ARSecurityLevelReasonable)
	if RenderForLog(mac) != mac {
		t.Fatal("not revealed at REASONABLE")
	}
	if RenderForLog("plain") != "plain" {
		t.Fatal("plain value changed")
	}
}

func TestInstallIdempotent(t *testing.T) {
	logger := InstallLogMasking(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if InstallLogMasking(logger) != logger {
		t.Fatal("not idempotent")
	}
	if _, ok := logger.Handler().(*SensitiveHandler); !ok {
		t.Fatal("handler not installed")
	}
}
