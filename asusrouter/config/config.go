// Package config ports `asusrouter/config` (`__init__.py` → config.go,
// `connection.py` → connection.go).
//
// Python raises KeyError for unknown keys; the Go `Set`/`Register`
// return an error instead and `Get` returns nil (`Lookup` returns the
// error).
package config

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// ARConfigKeyBase mirrors `ARConfigKeyBase`: the base of every
// configuration key enum.
type ARConfigKeyBase interface {
	// ConfigKey returns the key value.
	ConfigKey() string
}

// Converter converts a raw value to the stored configuration value.
type Converter func(any) any

// ARConfigKey mirrors `ARConfigKey` (StrEnum).
// Configuration keys for AsusRouter.
type ARConfigKey string

const (
	// Optimistic temperature
	ARConfigKeyOptimisticTemperature         ARConfigKey = "optimistic_temperature"
	ARConfigKeyNotifiedOptimisticTemperature ARConfigKey = "notified_optimistic_temperature"
	// Seed boot time; when set it anchors the first stabilization
	ARConfigKeyBoottime ARConfigKey = "boottime"
	// Data dump sensitive-data warning shown once per session
	ARConfigKeyNotifiedDump ARConfigKey = "notified_dump"
	// Probe report redaction warning shown once per session
	ARConfigKeyNotifiedProbe ARConfigKey = "notified_probe"
	// How many log entries are kept across reads
	ARConfigKeyLogHistoryLimit ARConfigKey = "log_history_limit"
	// Security level applied to logged data
	ARConfigKeySecurityLevelLog ARConfigKey = "security_level_log"
	// Security level applied to data exposed to consumers
	ARConfigKeySecurityLevelData ARConfigKey = "security_level_data"
)

// ARConfigKeyMembers lists the members in definition order.
var ARConfigKeyMembers = []ARConfigKey{
	ARConfigKeyOptimisticTemperature,
	ARConfigKeyNotifiedOptimisticTemperature,
	ARConfigKeyBoottime,
	ARConfigKeyNotifiedDump,
	ARConfigKeyNotifiedProbe,
	ARConfigKeyLogHistoryLimit,
	ARConfigKeySecurityLevelLog,
	ARConfigKeySecurityLevelData,
}

var aRConfigKeyNames = map[ARConfigKey]string{
	ARConfigKeyOptimisticTemperature:         "OPTIMISTIC_TEMPERATURE",
	ARConfigKeyNotifiedOptimisticTemperature: "NOTIFIED_OPTIMISTIC_TEMPERATURE",
	ARConfigKeyBoottime:                      "BOOTTIME",
	ARConfigKeyNotifiedDump:                  "NOTIFIED_DUMP",
	ARConfigKeyNotifiedProbe:                 "NOTIFIED_PROBE",
	ARConfigKeyLogHistoryLimit:               "LOG_HISTORY_LIMIT",
	ARConfigKeySecurityLevelLog:              "SECURITY_LEVEL_LOG",
	ARConfigKeySecurityLevelData:             "SECURITY_LEVEL_DATA",
}

// ConfigKey returns the key value.
func (e ARConfigKey) ConfigKey() string { return string(e) }

// Name returns the Python member name.
func (e ARConfigKey) Name() string { return aRConfigKeyNames[e] }

// Valid reports whether e is a defined member.
func (e ARConfigKey) Valid() bool { _, ok := aRConfigKeyNames[e]; return ok }

// Defaults
const (
	ConfigDefaultBool            = false
	ConfigDefaultInt             = 0
	ConfigDefaultAlreadyNotified = false
	ConfigDefaultLogHistory      = 50000
)

// SafeBoolConfig mirrors `safe_bool_config`: convert a value to a
// boolean, defaulting to ConfigDefaultBool.
func SafeBoolConfig(value any) bool {
	configValue := converters.RawToBool(value)

	if configValue == nil {
		return ConfigDefaultBool
	}

	return *configValue
}

// SafeIntConfig mirrors `safe_int_config`: convert a value to an
// integer, defaulting to ConfigDefaultInt.
func SafeIntConfig(value any) int {
	configValue := converters.RawToInt(value)

	if configValue == nil {
		return ConfigDefaultInt
	}

	return *configValue
}

// SafeDatetimeConfig mirrors `safe_datetime_config`: convert a value to
// a datetime (or parse a string), else None.
func SafeDatetimeConfig(value any) *time.Time {
	switch v := value.(type) {
	case time.Time:
		return &v
	case *time.Time:
		return v
	case string:
		return converters.RawToDatetime(v)
	}
	return nil
}

// SafeBoottimeConfig mirrors `safe_boottime_config`: convert a value to
// an aware boot time, else None.
func SafeBoottimeConfig(value any) *time.Time {
	configValue := SafeDatetimeConfig(value)

	if configValue != nil && configValue.Location() == converters.NaiveLocation {
		slog.Warn(
			"Ignoring the seeded boot time: it carries no timezone, " +
				"and the device reports its own with one",
		)
		return nil
	}

	return configValue
}

// Converter adapters storing plain values (nil for Python None)
var (
	safeBoolConverter Converter = func(v any) any { return SafeBoolConfig(v) }
	safeIntConverter  Converter = func(v any) any { return SafeIntConfig(v) }
	boottimeConverter Converter = func(v any) any {
		if t := SafeBoottimeConfig(v); t != nil {
			return *t
		}
		return nil
	}
	securityLevelConverter Converter = func(v any) any {
		return security.ARSecurityLevelFromValue(v)
	}
)

// SafeBoolConverter is SafeBoolConfig as a Converter (the default one).
var SafeBoolConverter = safeBoolConverter

// SafeIntConverter is SafeIntConfig as a Converter.
var SafeIntConverter = safeIntConverter

// ConfigDefault mirrors `CONFIG_DEFAULT`.
var ConfigDefault = map[ARConfigKey]any{
	// If set, the temperature will be automatically adjusted
	// to fit the expected range
	ARConfigKeyOptimisticTemperature:         ConfigDefaultBool,
	ARConfigKeyNotifiedOptimisticTemperature: ConfigDefaultAlreadyNotified,
	// If set, this boot time anchors the first stabilization on connect
	ARConfigKeyBoottime:      nil,
	ARConfigKeyNotifiedDump:  ConfigDefaultAlreadyNotified,
	ARConfigKeyNotifiedProbe: ConfigDefaultAlreadyNotified,
	// Log entries kept across reads; 0 keeps everything
	ARConfigKeyLogHistoryLimit: ConfigDefaultLogHistory,
	// Logs sanitize sensitive data by default
	ARConfigKeySecurityLevelLog: security.ARSecurityLevelSanitized,
	// Data exposes reasonably-sensitive values (MAC, IP) but not secrets
	ARConfigKeySecurityLevelData: security.ARSecurityLevelReasonable,
}

// TypesDefault mirrors `TYPES_DEFAULT`.
var TypesDefault = map[ARConfigKey]Converter{
	// Optimistic temperature
	ARConfigKeyOptimisticTemperature:         safeBoolConverter,
	ARConfigKeyNotifiedOptimisticTemperature: safeBoolConverter,
	// Seed boot time
	ARConfigKeyBoottime: boottimeConverter,
	// Data dump notification flag
	ARConfigKeyNotifiedDump: safeBoolConverter,
	// Probe report notification flag
	ARConfigKeyNotifiedProbe:   safeBoolConverter,
	ARConfigKeyLogHistoryLimit: safeIntConverter,
	// Security levels
	ARConfigKeySecurityLevelLog:  securityLevelConverter,
	ARConfigKeySecurityLevelData: securityLevelConverter,
}

// KeyError mirrors the Python KeyError raised for unknown keys.
type KeyError struct {
	Message string
}

func (e *KeyError) Error() string { return e.Message }

// ARConfigBase mirrors `ARConfigBase`: the thread-safe store shared by
// all configurations. Keys keep their registration order.
type ARConfigBase struct {
	mu      sync.Mutex
	options map[ARConfigKeyBase]any
	types   map[ARConfigKeyBase]Converter
	order   []ARConfigKeyBase
}

// NewARConfigBase creates an empty configuration.
func NewARConfigBase() *ARConfigBase {
	return &ARConfigBase{
		options: map[ARConfigKeyBase]any{},
		types:   map[ARConfigKeyBase]Converter{},
	}
}

// putLocked stores an option, keeping the insertion order.
func (c *ARConfigBase) putLocked(key ARConfigKeyBase, value any) {
	if c.options == nil {
		c.options = map[ARConfigKeyBase]any{}
		c.types = map[ARConfigKeyBase]Converter{}
	}
	if _, ok := c.options[key]; !ok {
		c.order = append(c.order, key)
	}
	c.options[key] = value
}

// Set mirrors `set`: set the configuration option. The key must be
// registered.
func (c *ARConfigBase) Set(key ARConfigKeyBase, value any) error {
	if key == nil {
		return &KeyError{fmt.Sprintf("Unknown configuration option: %v. Register it before setting.", key)}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.options[key]; !ok {
		return &KeyError{fmt.Sprintf(
			"Unknown configuration option: %s. Register it before setting.", key.ConfigKey(),
		)}
	}
	converter, ok := c.types[key]
	if !ok {
		converter = safeBoolConverter
	}
	c.options[key] = converter(value)
	return nil
}

// Lookup mirrors `get` including its KeyError.
func (c *ARConfigBase) Lookup(key ARConfigKeyBase) (any, error) {
	if key == nil {
		return nil, &KeyError{"Unknown configuration option: <nil>"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.options[key]
	if !ok {
		return nil, &KeyError{"Unknown configuration option: " + key.ConfigKey()}
	}
	return value, nil
}

// Get mirrors `get`: the configuration option, nil when unknown.
func (c *ARConfigBase) Get(key ARConfigKeyBase) any {
	value, _ := c.Lookup(key)
	return value
}

// Keys mirrors `keys`: the configuration keys in registration order.
func (c *ARConfigBase) Keys() []ARConfigKeyBase {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.order)
}

// ARConfigOption is one `(key, value)` pair of `list`.
type ARConfigOption struct {
	Key   ARConfigKeyBase
	Value any
}

// List mirrors `list`: all configuration options in registration order.
func (c *ARConfigBase) List() []ARConfigOption {
	c.mu.Lock()
	defer c.mu.Unlock()
	options := make([]ARConfigOption, 0, len(c.order))
	for _, key := range c.order {
		options = append(options, ARConfigOption{key, c.options[key]})
	}
	return options
}

// resetLocked drops all options and converters.
func (c *ARConfigBase) resetLocked() {
	c.options = map[ARConfigKeyBase]any{}
	c.types = map[ARConfigKeyBase]Converter{}
	c.order = nil
}

// Reset mirrors `reset`: remove all configuration options.
func (c *ARConfigBase) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()
}

// Register mirrors `register`: register a converter for a config key
// (nil means SafeBoolConfig) and store its default `converter(None)`.
func (c *ARConfigBase) Register(key ARConfigKeyBase, converter Converter) error {
	if key == nil {
		return &KeyError{"Unknown configuration key: <nil>"}
	}

	if converter == nil {
		converter = safeBoolConverter
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(key, converter(nil))
	c.types[key] = converter
	return nil
}

// EnsureNotificationFlag mirrors `ensure_notification_flag`: register a
// notification-flag key with its default if absent.
func (c *ARConfigBase) EnsureNotificationFlag(key ARConfigKeyBase) {
	if !c.Contains(key) {
		_ = c.Register(key, nil)
		_ = c.Set(key, ConfigDefaultAlreadyNotified)
	}
}

// Contains mirrors `__contains__`: check if a configuration key exists.
func (c *ARConfigBase) Contains(key ARConfigKeyBase) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.options[key]
	return ok
}

// Types mirrors the `types` property: a copy of the converters.
func (c *ARConfigBase) Types() map[ARConfigKeyBase]Converter {
	c.mu.Lock()
	defer c.mu.Unlock()
	types := make(map[ARConfigKeyBase]Converter, len(c.types))
	for key, converter := range c.types {
		types[key] = converter
	}
	return types
}

// ARGlobalConfig mirrors `ARGlobalConfig`: configuration class for
// AsusRouter.
type ARGlobalConfig struct {
	ARConfigBase
}

// NewARGlobalConfig mirrors `ARGlobalConfig(defaults)`.
func NewARGlobalConfig(defaults map[ARConfigKey]any) *ARGlobalConfig {
	c := &ARGlobalConfig{}
	c.Reset(defaults)
	return c
}

// Reset mirrors `reset`: reset all configuration options. Without
// defaults ConfigDefault applies; keys missing from defaults are None
// (values are stored as given, not converted).
func (c *ARGlobalConfig) Reset(defaults map[ARConfigKey]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()

	if len(defaults) == 0 {
		defaults = ConfigDefault
	}
	for _, key := range ARConfigKeyMembers {
		c.putLocked(key, defaults[key])
		converter, ok := TypesDefault[key]
		if !ok {
			converter = safeBoolConverter
		}
		c.types[key] = converter
	}
}

// ARConfig mirrors the global `ARConfig` instance.
var ARConfig = NewARGlobalConfig(nil)

// ARInstanceConfig mirrors `ARInstanceConfig`: configuration class for
// AsusRouter instances.
//
// These configurations are used to overwrite the global configuration.
// If the search key is present in the instance configuration, AR will
// use it when needed.
type ARInstanceConfig struct {
	ARConfigBase
}

// NewARInstanceConfig mirrors `ARInstanceConfig(defaults)`.
func NewARInstanceConfig(defaults map[ARConfigKey]any) *ARInstanceConfig {
	c := &ARInstanceConfig{}
	c.Reset(defaults)
	return c
}

// Reset mirrors `reset`: reset all configuration options using the
// provided defaults.
//
// This method removes all the existing configuration options and
// converters. Each default is stored after conversion with its default
// converter (SafeBoolConfig for keys without one).
func (c *ARInstanceConfig) Reset(defaults map[ARConfigKey]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()

	// Keep the key definition order (Python keeps the dict order)
	for _, key := range ARConfigKeyMembers {
		value, ok := defaults[key]
		if !ok {
			continue
		}
		// Check if a default converter is available
		converter, ok := TypesDefault[key]
		if !ok {
			converter = safeBoolConverter
		}
		// Save the converter and safe apply the value
		c.putLocked(key, converter(value))
		c.types[key] = converter
	}
}

// Lookup mirrors `get`: the instance option, falling back to the global
// ARConfig. A stored nil does not fall through.
func (c *ARInstanceConfig) Lookup(key ARConfigKeyBase) (any, error) {
	c.mu.Lock()
	value, ok := c.options[key]
	c.mu.Unlock()

	if ok {
		return value, nil
	}

	// Defer to global configuration
	return ARConfig.Lookup(key)
}

// Get mirrors `get`: the instance option, falling back to the global
// ARConfig; nil when unknown.
func (c *ARInstanceConfig) Get(key ARConfigKeyBase) any {
	value, _ := c.Lookup(key)
	return value
}

// Remove mirrors `remove`: remove configuration option of the instance.
func (c *ARInstanceConfig) Remove(key ARConfigKeyBase) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.options[key]; ok {
		delete(c.options, key)
		delete(c.types, key)
		c.order = slices.DeleteFunc(c.order, func(k ARConfigKeyBase) bool { return k == key })
	}
}

// Go-only typed accessors

// GetBool returns a boolean option (false when unknown or not a bool).
func GetBool(c interface{ Get(ARConfigKeyBase) any }, key ARConfigKeyBase) bool {
	value, _ := c.Get(key).(bool)
	return value
}

// GetInt returns an integer option (0 when unknown or not an int).
func GetInt(c interface{ Get(ARConfigKeyBase) any }, key ARConfigKeyBase) int {
	value, _ := c.Get(key).(int)
	return value
}

// GetSecurityLevel returns a security level option (UNKNOWN when unknown).
func GetSecurityLevel(c interface{ Get(ARConfigKeyBase) any }, key ARConfigKeyBase) security.ARSecurityLevel {
	if value, ok := c.Get(key).(security.ARSecurityLevel); ok {
		return value
	}
	return security.ARSecurityLevelUnknown
}
