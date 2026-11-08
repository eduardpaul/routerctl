package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const testInstanceDefaultBool = !ConfigDefaultBool

var testInstanceDefaults = map[ARConfigKey]any{
	ARConfigKeyOptimisticTemperature: testInstanceDefaultBool,
}

func TestInstanceReset(t *testing.T) {
	resetConfig(t)
	inst := NewARInstanceConfig(testInstanceDefaults)
	if inst.Get(ARConfigKeyOptimisticTemperature) != testInstanceDefaultBool {
		t.Fatal("instance value not applied")
	}
	if ARConfig.Get(ARConfigKeyOptimisticTemperature) != ConfigDefault[ARConfigKeyOptimisticTemperature] {
		t.Fatal("global changed")
	}
}

func TestInstanceGet(t *testing.T) {
	resetConfig(t)
	inst := NewARInstanceConfig(nil)
	if inst.Get(ARConfigKeyOptimisticTemperature) != ConfigDefault[ARConfigKeyOptimisticTemperature] {
		t.Fatal("global value not returned")
	}
	if GetSecurityLevel(inst, ARConfigKeySecurityLevelLog) != security.ARSecurityLevelSanitized {
		t.Fatal("global security level not returned")
	}
}

func TestInstanceConvertsDefaults(t *testing.T) {
	inst := NewARInstanceConfig(map[ARConfigKey]any{
		ARConfigKeySecurityLevelLog: "strict",
		ARConfigKeyLogHistoryLimit:  "10",
	})
	if inst.Get(ARConfigKeySecurityLevelLog) != security.ARSecurityLevelStrict || inst.Get(ARConfigKeyLogHistoryLimit) != 10 {
		t.Fatal("defaults not converted")
	}
}

func TestInstanceStoredNilDoesNotFallThrough(t *testing.T) {
	resetConfig(t)
	inst := NewARInstanceConfig(map[ARConfigKey]any{ARConfigKeyBoottime: "bad"})
	if !inst.Contains(ARConfigKeyBoottime) || inst.Get(ARConfigKeyBoottime) != nil {
		t.Fatal("stored None must be returned")
	}
}

func TestInstanceRemove(t *testing.T) {
	resetConfig(t)
	inst := NewARInstanceConfig(testInstanceDefaults)
	inst.Remove(ARConfigKeyOptimisticTemperature)
	if inst.Get(ARConfigKeyOptimisticTemperature) != ConfigDefault[ARConfigKeyOptimisticTemperature] {
		t.Fatal("global value not returned after remove")
	}
	if len(inst.Keys()) != 0 {
		t.Fatal("key not removed")
	}
}

func TestInstanceSetNonexistent(t *testing.T) {
	resetConfig(t)
	inst := NewARInstanceConfig(nil)
	err := inst.Set(ARConfigKeyOptimisticTemperature, true)
	var keyErr *KeyError
	if !errors.As(err, &keyErr) || !strings.Contains(err.Error(), "Register it before setting.") {
		t.Fatalf("err = %v", err)
	}
	if inst.Get(ARConfigKeyOptimisticTemperature) != ConfigDefault[ARConfigKeyOptimisticTemperature] {
		t.Fatal("global value not returned")
	}
}

func TestInstanceRegisterAndSet(t *testing.T) {
	resetConfig(t)
	inst := NewARInstanceConfig(nil)
	_ = inst.Register(ARConfigKeyOptimisticTemperature, SafeBoolConverter)
	_ = inst.Set(ARConfigKeyOptimisticTemperature, testInstanceDefaultBool)
	if inst.Get(ARConfigKeyOptimisticTemperature) != testInstanceDefaultBool {
		t.Fatal("instance value not returned")
	}
	if ARConfig.Get(ARConfigKeyOptimisticTemperature) != ConfigDefault[ARConfigKeyOptimisticTemperature] {
		t.Fatal("global changed")
	}
}

func TestInstanceRegisterDoesNotChangeGlobal(t *testing.T) {
	resetConfig(t)
	inst := NewARInstanceConfig(nil)
	_ = inst.Register(ARConfigKeyOptimisticTemperature, SafeBoolConverter)
	if inst.Get(ARConfigKeyOptimisticTemperature) != ConfigDefault[ARConfigKeyOptimisticTemperature] {
		t.Fatal("unexpected value")
	}
}

func TestInstanceGlobalDynamic(t *testing.T) {
	resetConfig(t)
	inst := NewARInstanceConfig(nil)
	globalDefault := ConfigDefault[ARConfigKeyOptimisticTemperature].(bool)
	if inst.Get(ARConfigKeyOptimisticTemperature) != globalDefault {
		t.Fatal("global value not returned")
	}
	_ = ARConfig.Set(ARConfigKeyOptimisticTemperature, !globalDefault)
	if inst.Get(ARConfigKeyOptimisticTemperature) != !globalDefault {
		t.Fatal("global change not reflected")
	}
}

func TestInstanceIndependence(t *testing.T) {
	inst1 := NewARInstanceConfig(testInstanceDefaults)
	inst2 := NewARInstanceConfig(testInstanceDefaults)
	_ = inst1.Set(ARConfigKeyOptimisticTemperature, !testInstanceDefaultBool)
	if inst1.Get(ARConfigKeyOptimisticTemperature) == testInstanceDefaultBool {
		t.Fatal("instance 1 not changed")
	}
	if inst2.Get(ARConfigKeyOptimisticTemperature) != testInstanceDefaultBool {
		t.Fatal("instance 2 affected")
	}
}
