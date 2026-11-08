// Package log ports `asusrouter/tools/security/log.py`: automatic masking
// of sensitive data in log records.
//
// It is a subpackage of `security` because it needs `config`, which
// itself needs `security.ARSecurityLevel` (Python hides that cycle).
//
// Go has no logger hierarchy, so the Python passive handler + filter
// become a `slog.Handler` wrapper (SensitiveHandler). Importing this
// package also points `security.LogLevel` (used by every sensitive
// type's `LogValue`) at EffectiveLogLevel.
package log

import (
	"context"
	"log/slog"
	"sync"
	"weak"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Live instance configs whose log level constrains the shared log masking.
// Weak so dead instances drop out on their own.
var (
	logConfigs     []weak.Pointer[config.ARInstanceConfig]
	logConfigsLock sync.Mutex
)

func init() {
	security.SetLogLevelFunc(EffectiveLogLevel)
}

// RegisterLogConfig mirrors `register_log_config`: register an instance
// config to constrain shared log masking.
func RegisterLogConfig(cfg *config.ARInstanceConfig) {
	logConfigsLock.Lock()
	defer logConfigsLock.Unlock()
	for _, p := range logConfigs {
		if p.Value() == cfg {
			return
		}
	}
	logConfigs = append(logConfigs, weak.Make(cfg))
}

// UnregisterLogConfig mirrors `unregister_log_config`: remove an instance
// config from the log masking constraint.
func UnregisterLogConfig(cfg *config.ARInstanceConfig) {
	logConfigsLock.Lock()
	defer logConfigsLock.Unlock()
	kept := logConfigs[:0]
	for _, p := range logConfigs {
		if v := p.Value(); v != nil && v != cfg {
			kept = append(kept, p)
		}
	}
	logConfigs = kept
}

// EffectiveLogLevel mirrors `effective_log_level`: return the most
// restrictive log level across global and instances.
//
// A record carries no instance identity, so the shared filter cannot know
// which instance emitted it. The strictest (lowest) level wins so that no
// instance's data leaks through a laxer one.
func EffectiveLogLevel() security.ARSecurityLevel {
	level := config.GetSecurityLevel(config.ARConfig, config.ARConfigKeySecurityLevelLog)

	logConfigsLock.Lock()
	configs := make([]*config.ARInstanceConfig, 0, len(logConfigs))
	for _, p := range logConfigs {
		if v := p.Value(); v != nil {
			configs = append(configs, v)
		}
	}
	logConfigsLock.Unlock()

	for _, cfg := range configs {
		instanceLevel := config.GetSecurityLevel(cfg, config.ARConfigKeySecurityLevelLog)
		level = min(level, instanceLevel)
	}

	return level
}

// RenderForLog mirrors `render_for_log`: render a value against the
// current effective log level.
//
// Use when a sensitive value must be embedded into a string (e.g. an
// error message) that will later be logged: the log handler cannot
// reach values already formatted into text, so mask them beforehand.
func RenderForLog(value any) any {
	return security.Render(value, EffectiveLogLevel())
}

// SensitiveHandler mirrors `SensitiveFilter` + `_PassiveHandler`: it
// renders sensitive attribute values per the log security level before
// passing the record to the wrapped handler.
type SensitiveHandler struct {
	next slog.Handler
}

// NewSensitiveHandler wraps a handler with sensitive-data masking.
func NewSensitiveHandler(next slog.Handler) *SensitiveHandler {
	if h, ok := next.(*SensitiveHandler); ok {
		return h
	}
	return &SensitiveHandler{next: next}
}

// Enabled reports whether the wrapped handler handles the level.
func (h *SensitiveHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// sensitiveOf returns the sensitive value an attribute value carries.
func sensitiveOf(v slog.Value) (security.ARSensitive, bool) {
	switch v.Kind() {
	case slog.KindLogValuer:
		s, ok := v.LogValuer().(security.ARSensitive)
		return s, ok
	case slog.KindAny:
		s, ok := v.Any().(security.ARSensitive)
		return s, ok
	}
	return nil, false
}

func hasSensitive(attrs []slog.Attr) bool {
	for _, a := range attrs {
		if _, ok := sensitiveOf(a.Value); ok {
			return true
		}
		if a.Value.Kind() == slog.KindGroup && hasSensitive(a.Value.Group()) {
			return true
		}
	}
	return false
}

func renderAttrs(attrs []slog.Attr, level security.ARSecurityLevel) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		switch s, ok := sensitiveOf(a.Value); {
		case ok:
			a.Value = slog.StringValue(security.RenderString(s, level))
		case a.Value.Kind() == slog.KindGroup:
			a.Value = slog.GroupValue(renderAttrs(a.Value.Group(), level)...)
		}
		out[i] = a
	}
	return out
}

// Handle masks sensitive attributes in place and passes the record on.
func (h *SensitiveHandler) Handle(ctx context.Context, record slog.Record) error {
	attrs := make([]slog.Attr, 0, record.NumAttrs())
	record.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	// Most records carry nothing sensitive; skip the level lookup then
	if !hasSensitive(attrs) {
		return h.next.Handle(ctx, record)
	}

	masked := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	masked.AddAttrs(renderAttrs(attrs, EffectiveLogLevel())...)
	return h.next.Handle(ctx, masked)
}

// WithAttrs masks the attributes (at the current level) and returns a
// wrapped handler.
func (h *SensitiveHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if hasSensitive(attrs) {
		attrs = renderAttrs(attrs, EffectiveLogLevel())
	}
	return &SensitiveHandler{next: h.next.WithAttrs(attrs)}
}

// WithGroup returns a wrapped handler with the group.
func (h *SensitiveHandler) WithGroup(name string) slog.Handler {
	return &SensitiveHandler{next: h.next.WithGroup(name)}
}

// InstallLogMasking mirrors `install_log_masking`: attach sensitive-data
// masking to a logger (nil means slog.Default(), which then is replaced).
//
// Idempotent: a logger that already masks is returned as is.
func InstallLogMasking(logger *slog.Logger) *slog.Logger {
	setDefault := logger == nil
	if setDefault {
		logger = slog.Default()
	}
	if _, ok := logger.Handler().(*SensitiveHandler); ok {
		return logger
	}

	masked := slog.New(NewSensitiveHandler(logger.Handler()))
	if setDefault {
		slog.SetDefault(masked)
	}
	return masked
}

// resetLogMasking mirrors `_reset_log_masking` (for tests): forget all
// registered instance configs.
func resetLogMasking() {
	logConfigsLock.Lock()
	defer logConfigsLock.Unlock()
	logConfigs = nil
}
