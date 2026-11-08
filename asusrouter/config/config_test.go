package config

import (
	"bytes"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

type mockKey string

func (k mockKey) ConfigKey() string { return string(k) }

const (
	mockKeyExisting mockKey = "existing"
	mockKeyAny      mockKey = "any"
	mockKeySome     mockKey = "some"
)

var keysBool = []ARConfigKey{
	ARConfigKeyOptimisticTemperature,
	ARConfigKeyNotifiedOptimisticTemperature,
}

// resetConfig resets the configuration before each test
func resetConfig(t *testing.T) {
	t.Helper()
	ARConfig.Reset(nil)
	t.Cleanup(func() { ARConfig.Reset(nil) })
}

func TestSafeDatetimeConfig(t *testing.T) {
	aware := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := SafeDatetimeConfig(aware); got == nil || !got.Equal(aware) {
		t.Errorf("datetime: %v", got)
	}
	for _, v := range []any{"not-a-date", 5, nil} {
		if got := SafeDatetimeConfig(v); got != nil {
			t.Errorf("SafeDatetimeConfig(%#v) = %v", v, got)
		}
	}
}

func TestSafeBoottimeConfigKeepsAware(t *testing.T) {
	aware := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := SafeBoottimeConfig(aware); got == nil || !got.Equal(aware) {
		t.Errorf("aware: %v", got)
	}
	want := time.Date(2026, 7, 31, 10, 24, 4, 0, time.FixedZone("", 7200))
	got := SafeBoottimeConfig("Fri, 31 Jul 2026 10:24:04 +0200")
	if got == nil || !got.Equal(want) {
		t.Errorf("aware_str: %v", got)
	}
	if _, off := got.Zone(); off != 7200 {
		t.Errorf("offset %d", off)
	}
	if SafeBoottimeConfig(nil) != nil {
		t.Error("none")
	}
}

func TestSafeBoottimeConfigDropsNaive(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)

	naive := time.Date(2026, 1, 1, 0, 0, 0, 0, converters.NaiveLocation)
	if SafeBoottimeConfig(naive) != nil {
		t.Fatal("naive boot time kept")
	}
	if !strings.Contains(buf.String(), "no timezone") {
		t.Fatalf("missing warning: %q", buf.String())
	}
}

func TestBoottimeDefaultNone(t *testing.T) {
	resetConfig(t)
	if ARConfig.Get(ARConfigKeyBoottime) != nil {
		t.Fatal("boottime default is not None")
	}
}

func TestEnsureNotificationFlag(t *testing.T) {
	for _, c := range []struct {
		key    mockKey
		called bool
	}{
		{mockKeyExisting, false}, {mockKeyAny, true}, {mockKeySome, true},
	} {
		config := NewARConfigBase()
		_ = config.Register(mockKeyExisting, func(any) any { return "kept" })
		config.EnsureNotificationFlag(c.key)
		value := config.Get(c.key)
		if c.called && value != ConfigDefaultAlreadyNotified {
			t.Errorf("%s: %v", c.key, value)
		}
		if !c.called && value != "kept" {
			t.Errorf("%s was overwritten: %v", c.key, value)
		}
	}
}

func TestEnsureNotificationFlagRegistersDefault(t *testing.T) {
	config := NewARConfigBase()
	if config.Contains(mockKeySome) {
		t.Fatal("unexpected key")
	}
	config.EnsureNotificationFlag(mockKeySome)
	if !config.Contains(mockKeySome) || config.Get(mockKeySome) != ConfigDefaultAlreadyNotified {
		t.Fatal("flag not registered with its default")
	}
}

func TestConfigCustomDefaults(t *testing.T) {
	config := NewARGlobalConfig(map[ARConfigKey]any{ARConfigKeyOptimisticTemperature: true})
	if config.Get(ARConfigKeyOptimisticTemperature) != true {
		t.Fatal("custom default not applied")
	}
}

func TestConfigWrongKey(t *testing.T) {
	resetConfig(t)
	for _, key := range []ARConfigKeyBase{ARConfigKey("unknown_option"), mockKeyAny, nil} {
		if err := ARConfig.Set(key, true); err == nil {
			t.Errorf("Set(%v) did not fail", key)
		}
		if _, err := ARConfig.Lookup(key); err == nil {
			t.Errorf("Lookup(%v) did not fail", key)
		}
		if ARConfig.Get(key) != nil {
			t.Errorf("Get(%v) is not nil", key)
		}
	}
}

func TestConfigContains(t *testing.T) {
	resetConfig(t)
	for _, key := range keysBool {
		if !ARConfig.Contains(key) {
			t.Errorf("%s missing", key)
		}
	}
	if ARConfig.Contains(ARConfigKey("not_a_key")) {
		t.Error("unexpected key")
	}
}

func TestGetValidKeyNotRegistered(t *testing.T) {
	_, err := NewARConfigBase().Lookup(ARConfigKeyOptimisticTemperature)
	if err == nil || !strings.Contains(err.Error(), "Unknown configuration option") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigKeys(t *testing.T) {
	resetConfig(t)
	keys := ARConfig.Keys()
	if len(keys) != len(ARConfigKeyMembers) {
		t.Fatalf("keys = %v", keys)
	}
	for i, key := range keys {
		if key != ARConfigKeyMembers[i] {
			t.Errorf("key %d = %v", i, key)
		}
	}
	// The keys list is a copy
	keys[0] = mockKeyAny
	if ARConfig.Keys()[0] != ARConfigKeyMembers[0] {
		t.Error("keys are not a copy")
	}
}

func TestConfigListAndTypes(t *testing.T) {
	resetConfig(t)
	options := ARConfig.List()
	if len(options) != len(ARConfigKeyMembers) || options[5].Key != ARConfigKeyLogHistoryLimit || options[5].Value != ConfigDefaultLogHistory {
		t.Fatalf("list = %v", options)
	}
	types := ARConfig.Types()
	if len(types) != len(ARConfigKeyMembers) {
		t.Fatalf("types = %v", types)
	}
	for key, converter := range types {
		if _, ok := key.(ARConfigKey); !ok || converter == nil {
			t.Errorf("bad type entry %v", key)
		}
	}
}

func TestRegister(t *testing.T) {
	resetConfig(t)
	intToBool := func(val any) any {
		v, _ := val.(int)
		return v != 0
	}
	_ = ARConfig.Register(ARConfigKeyOptimisticTemperature, intToBool)
	_ = ARConfig.Set(ARConfigKeyOptimisticTemperature, 1)
	if ARConfig.Get(ARConfigKeyOptimisticTemperature) != true {
		t.Fatal("register converter not used")
	}
	_ = ARConfig.Set(ARConfigKeyOptimisticTemperature, 0)
	if ARConfig.Get(ARConfigKeyOptimisticTemperature) != false {
		t.Fatal("register converter not used")
	}
}

func TestRegisterOverwritesExisting(t *testing.T) {
	resetConfig(t)
	_ = ARConfig.Register(ARConfigKeyOptimisticTemperature, func(any) any { return true })
	_ = ARConfig.Set(ARConfigKeyOptimisticTemperature, "anything")
	if ARConfig.Get(ARConfigKeyOptimisticTemperature) != true {
		t.Fatal("always_true")
	}
	_ = ARConfig.Register(ARConfigKeyOptimisticTemperature, func(any) any { return false })
	_ = ARConfig.Set(ARConfigKeyOptimisticTemperature, "anything")
	if ARConfig.Get(ARConfigKeyOptimisticTemperature) != false {
		t.Fatal("always_false")
	}
}

func TestRegisterUnknownKey(t *testing.T) {
	if err := ARConfig.Register(nil, func(x any) any { return x }); err == nil {
		t.Fatal("expected KeyError")
	}
}

func TestRegisterNoConverter(t *testing.T) {
	resetConfig(t)
	_ = ARConfig.Register(ARConfigKeyOptimisticTemperature, nil)
	_ = ARConfig.Set(ARConfigKeyOptimisticTemperature, 1)
	if ARConfig.Get(ARConfigKeyOptimisticTemperature) != true {
		t.Fatal("bool conversion not used")
	}
	_ = ARConfig.Set(ARConfigKeyOptimisticTemperature, 0)
	if ARConfig.Get(ARConfigKeyOptimisticTemperature) != false {
		t.Fatal("bool conversion not used")
	}
}

func TestReset(t *testing.T) {
	resetConfig(t)
	for _, key := range keysBool {
		_ = ARConfig.Set(key, !ConfigDefaultBool)
		if ARConfig.Get(key) == ConfigDefaultBool {
			t.Fatal("set failed")
		}
		_ = ARConfig.Register(key, func(x any) any { return x == nil })
	}
	ARConfig.Reset(nil)
	for _, key := range keysBool {
		if ARConfig.Get(key) != ConfigDefaultBool {
			t.Errorf("%s not reset", key)
		}
		if ARConfig.Types()[key](true) != SafeBoolConfig(true) || ARConfig.Types()[key]("off") != false {
			t.Errorf("%s converter not reset", key)
		}
	}
}

func TestThreadSafety(t *testing.T) {
	resetConfig(t)
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			key := keysBool[rand.Intn(len(keysBool))]
			_ = ARConfig.Set(key, rand.Intn(2) == 1)
			_ = ARConfig.Get(key)
		}()
		go func() {
			defer wg.Done()
			_ = ARConfig.Register(ARConfigKeyOptimisticTemperature, SafeBoolConverter)
		}()
		go func() {
			defer wg.Done()
			ARConfig.Reset(nil)
		}()
	}
	wg.Wait()
}

func TestSafeBoolConfig(t *testing.T) {
	cases := []struct {
		value    any
		expected bool
	}{
		{true, true}, {false, false}, {"true", true}, {"false", false},
		{1, true}, {0, false}, {"1", true}, {"0", false}, {"string", false}, {nil, false},
	}
	for _, c := range cases {
		if got := SafeBoolConfig(c.value); got != c.expected {
			t.Errorf("SafeBoolConfig(%#v) = %v", c.value, got)
		}
	}
}

func TestSafeIntConfig(t *testing.T) {
	cases := []struct {
		value    any
		expected int
	}{
		{1, 1}, {0, 0}, {"1", 1}, {"0", 0}, {"string", 0}, {nil, 0},
	}
	for _, c := range cases {
		if got := SafeIntConfig(c.value); got != c.expected {
			t.Errorf("SafeIntConfig(%#v) = %v", c.value, got)
		}
	}
}

func TestBoolConfig(t *testing.T) {
	resetConfig(t)
	for _, key := range keysBool {
		if ARConfig.Get(key) != ConfigDefaultBool {
			t.Errorf("default %s", key)
		}
		for _, value := range []bool{true, false} {
			_ = ARConfig.Set(key, value)
			if ARConfig.Get(key) != value || GetBool(ARConfig, key) != value {
				t.Errorf("set %s = %v", key, value)
			}
		}
	}
}
