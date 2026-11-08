// Connection configuration (connection.py).
//
// This module handles the configuration of the connection settings
// for the library.

package config

// ARConnectionConfigKey mirrors `ARConnectionConfigKey` (StrEnum).
// Connection configuration keys for AsusRouter.
type ARConnectionConfigKey string

const (
	// Allow automatic fallback
	ARConnectionConfigKeyAllowFallback ARConnectionConfigKey = "allow_fallback"
	// Allow multiple fallbacks
	ARConnectionConfigKeyAllowMultipleFallbacks ARConnectionConfigKey = "allow_multiple_fallbacks"
	// Allow upgrade from HTTP to HTTPS
	ARConnectionConfigKeyAllowUpgradeHttpToHttps ARConnectionConfigKey = "allow_upgrade_http_to_https"
	// Force legacy system status (skip the modern diagnostics endpoint)
	ARConnectionConfigKeyForceLegacySystemStatus ARConnectionConfigKey = "force_legacy_system_status"
	// Max concurrent HTTP requests (read once at connection init)
	ARConnectionConfigKeyMaxConcurrentRequests ARConnectionConfigKey = "max_concurrent_requests"
	// Port
	ARConnectionConfigKeyPort ARConnectionConfigKey = "port"
	// Strict SSL
	ARConnectionConfigKeyStrictSsl ARConnectionConfigKey = "strict_ssl"
	// Use SSL
	ARConnectionConfigKeyUseSsl ARConnectionConfigKey = "use_ssl"
	// Verify SSL certificate
	ARConnectionConfigKeyVerifySsl ARConnectionConfigKey = "verify_ssl"
)

// ARConnectionConfigKeyMembers lists the members in definition order.
var ARConnectionConfigKeyMembers = []ARConnectionConfigKey{
	ARConnectionConfigKeyAllowFallback,
	ARConnectionConfigKeyAllowMultipleFallbacks,
	ARConnectionConfigKeyAllowUpgradeHttpToHttps,
	ARConnectionConfigKeyForceLegacySystemStatus,
	ARConnectionConfigKeyMaxConcurrentRequests,
	ARConnectionConfigKeyPort,
	ARConnectionConfigKeyStrictSsl,
	ARConnectionConfigKeyUseSsl,
	ARConnectionConfigKeyVerifySsl,
}

var aRConnectionConfigKeyNames = map[ARConnectionConfigKey]string{
	ARConnectionConfigKeyAllowFallback:           "ALLOW_FALLBACK",
	ARConnectionConfigKeyAllowMultipleFallbacks:  "ALLOW_MULTIPLE_FALLBACKS",
	ARConnectionConfigKeyAllowUpgradeHttpToHttps: "ALLOW_UPGRADE_HTTP_TO_HTTPS",
	ARConnectionConfigKeyForceLegacySystemStatus: "FORCE_LEGACY_SYSTEM_STATUS",
	ARConnectionConfigKeyMaxConcurrentRequests:   "MAX_CONCURRENT_REQUESTS",
	ARConnectionConfigKeyPort:                    "PORT",
	ARConnectionConfigKeyStrictSsl:               "STRICT_SSL",
	ARConnectionConfigKeyUseSsl:                  "USE_SSL",
	ARConnectionConfigKeyVerifySsl:               "VERIFY_SSL",
}

// ConfigKey returns the key value.
func (e ARConnectionConfigKey) ConfigKey() string { return string(e) }

// Name returns the Python member name.
func (e ARConnectionConfigKey) Name() string { return aRConnectionConfigKeyNames[e] }

// Valid reports whether e is a defined member.
func (e ARConnectionConfigKey) Valid() bool { _, ok := aRConnectionConfigKeyNames[e]; return ok }

// ConnectionConfigDefault mirrors `CONNECTION_CONFIG_DEFAULT`.
var ConnectionConfigDefault = map[ARConnectionConfigKey]any{
	// Allow automatic fallback
	ARConnectionConfigKeyAllowFallback: ConfigDefaultBool,
	// Allow multiple fallbacks
	// If set, AsusRouter will allow fallback to a different connection method
	// any number of times
	// (except for consecutive attempts, where loop protection will trigger)
	ARConnectionConfigKeyAllowMultipleFallbacks: ConfigDefaultBool,
	// Allow upgrade from HTTP to HTTPS
	ARConnectionConfigKeyAllowUpgradeHttpToHttps: true,
	// If set, system status always uses legacy appGet cpu/ram data
	ARConnectionConfigKeyForceLegacySystemStatus: ConfigDefaultBool,
	// Max concurrent HTTP requests; 1 keeps requests fully serialized
	ARConnectionConfigKeyMaxConcurrentRequests: 1,
	// Port
	ARConnectionConfigKeyPort: ConfigDefaultInt,
	// If set, AsusRouter will not allow falling back to a non-SSL connection
	// automatically.
	ARConnectionConfigKeyStrictSsl: ConfigDefaultBool,
	// Use SSL
	ARConnectionConfigKeyUseSsl: ConfigDefaultBool,
	// If set, AsusRouter will verify SSL certificates when connecting
	// via HTTPS. Most Asus routers use self-signed certificates, so
	// connections may fail unless you manually add the issuer to your
	// trusted certificates.
	ARConnectionConfigKeyVerifySsl: ConfigDefaultBool,
}

// ConnectionConfigTypesDefaults mirrors `CONNECTION_CONFIG_TYPES_DEFAULTS`.
var ConnectionConfigTypesDefaults = map[ARConnectionConfigKey]Converter{
	// Allow automatic fallback
	ARConnectionConfigKeyAllowFallback: safeBoolConverter,
	// Allow multiple fallbacks
	ARConnectionConfigKeyAllowMultipleFallbacks: safeBoolConverter,
	// Allow upgrade from HTTP to HTTPS
	ARConnectionConfigKeyAllowUpgradeHttpToHttps: safeBoolConverter,
	// Force legacy system status
	ARConnectionConfigKeyForceLegacySystemStatus: safeBoolConverter,
	// Max concurrent HTTP requests
	ARConnectionConfigKeyMaxConcurrentRequests: safeIntConverter,
	// Port
	ARConnectionConfigKeyPort: safeIntConverter,
	// Strict SSL
	ARConnectionConfigKeyStrictSsl: safeBoolConverter,
	// Use SSL
	ARConnectionConfigKeyUseSsl: safeBoolConverter,
	// Verify SSL certificate
	ARConnectionConfigKeyVerifySsl: safeBoolConverter,
}

// ARConnectionConfig mirrors `ARConnectionConfig`: connection
// configuration for AsusRouter.
type ARConnectionConfig struct {
	ARConfigBase
}

// NewARConnectionConfig mirrors `ARConnectionConfig(defaults=None)`; at
// most one defaults map is used.
func NewARConnectionConfig(defaults ...map[ARConnectionConfigKey]any) *ARConnectionConfig {
	c := &ARConnectionConfig{}
	var d map[ARConnectionConfigKey]any
	if len(defaults) > 0 {
		d = defaults[0]
	}
	c.Reset(d)
	return c
}

// Reset mirrors `reset`: reset all connection configuration options.
// Without defaults ConnectionConfigDefault applies; keys missing from
// defaults are None (values are stored as given, not converted).
func (c *ARConnectionConfig) Reset(defaults map[ARConnectionConfigKey]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()

	if len(defaults) == 0 {
		defaults = ConnectionConfigDefault
	}
	for _, key := range ARConnectionConfigKeyMembers {
		c.putLocked(key, defaults[key])
		converter, ok := ConnectionConfigTypesDefaults[key]
		if !ok {
			converter = safeBoolConverter
		}
		c.types[key] = converter
	}
}
