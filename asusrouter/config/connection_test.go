package config

import "testing"

var keysInt = []ARConnectionConfigKey{ARConnectionConfigKeyPort}

func TestConnectionAllKeysPresent(t *testing.T) {
	config := NewARConnectionConfig()
	for _, key := range ARConnectionConfigKeyMembers {
		if !config.Contains(key) {
			t.Errorf("%s missing", key)
		}
	}
}

func TestConnectionDefaultValue(t *testing.T) {
	cases := []struct {
		key      ARConnectionConfigKey
		expected any
	}{
		{ARConnectionConfigKeyAllowFallback, ConfigDefaultBool},
		{ARConnectionConfigKeyAllowMultipleFallbacks, ConfigDefaultBool},
		{ARConnectionConfigKeyAllowUpgradeHttpToHttps, true},
		{ARConnectionConfigKeyMaxConcurrentRequests, 1},
		{ARConnectionConfigKeyPort, ConfigDefaultInt},
		{ARConnectionConfigKeyStrictSsl, ConfigDefaultBool},
		{ARConnectionConfigKeyUseSsl, ConfigDefaultBool},
		{ARConnectionConfigKeyVerifySsl, ConfigDefaultBool},
	}
	config := NewARConnectionConfig()
	for _, c := range cases {
		if got := config.Get(c.key); got != c.expected {
			t.Errorf("%s = %v, want %v", c.key, got, c.expected)
		}
	}
}

func TestConnectionCustomDefaults(t *testing.T) {
	config := NewARConnectionConfig(map[ARConnectionConfigKey]any{ARConnectionConfigKeyUseSsl: true})
	if config.Get(ARConnectionConfigKeyUseSsl) != true {
		t.Fatal("custom default not applied")
	}
}

func TestConnectionDefaultsMatchConstant(t *testing.T) {
	config := NewARConnectionConfig()
	for key, expected := range ConnectionConfigDefault {
		if config.Get(key) != expected {
			t.Errorf("%s = %v", key, config.Get(key))
		}
	}
}

func TestConnectionIntConfig(t *testing.T) {
	for _, key := range keysInt {
		configs := NewARConnectionConfig()
		if configs.Get(key) != ConfigDefaultInt {
			t.Errorf("default %s", key)
		}
		for _, value := range []int{1, 2, 3} {
			_ = configs.Set(key, value)
			if configs.Get(key) != value || GetInt(configs, key) != value {
				t.Errorf("set %s = %d", key, value)
			}
		}
		_ = configs.Set(key, "8443")
		if configs.Get(key) != 8443 {
			t.Errorf("string not converted for %s", key)
		}
	}
}
