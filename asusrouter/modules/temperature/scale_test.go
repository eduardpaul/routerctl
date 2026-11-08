package temperature

import (
	"bytes"
	"log/slog"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/config"
)

// Ported from tests/modules/temperature/test_scale.py.

func fptr(v float64) *float64 { return &v }

func TestScaleTemperature(t *testing.T) {
	cases := []struct {
		name        string
		temperature map[string]*float64
		want        map[string]float64
		scaled      bool
	}{
		{"in_range", map[string]*float64{"a": fptr(30), "b": fptr(40)}, map[string]float64{"a": 30, "b": 40}, false},
		{"scale_up", map[string]*float64{"a": fptr(0.03), "b": fptr(0.04)}, map[string]float64{"a": 30, "b": 40}, true},
		{"scale_down", map[string]*float64{"a": fptr(30000), "b": fptr(40000)}, map[string]float64{"a": 30, "b": 40}, true},
		{"none_ignored", map[string]*float64{"a": nil}, map[string]float64{}, false},
		{"max_steps_up", map[string]*float64{"a": fptr(0.00003), "b": fptr(0.00004)}, map[string]float64{"a": 3, "b": 4}, true},
		{"max_steps_down", map[string]*float64{"a": fptr(30000000), "b": fptr(40000000)}, map[string]float64{"a": 300, "b": 400}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, scaled := ScaleTemperature(tc.temperature, ExpectedTemperatureMin, ExpectedTemperatureMax, MaxScaleCount)
			if !reflect.DeepEqual(got, tc.want) || scaled != tc.scaled {
				t.Errorf("got %v, %v", got, scaled)
			}
		})
	}

	for name, temperature := range map[string]map[string]*float64{
		"in_range":  {"a": fptr(30), "b": fptr(40)},
		"scale_up":  {"a": fptr(0.03)},
		"max_steps": {"a": fptr(0.00003)},
	} {
		t.Run("deterministic_"+name, func(t *testing.T) {
			first, s1 := ScaleTemperature(temperature, ExpectedTemperatureMin, ExpectedTemperatureMax, MaxScaleCount)
			second, s2 := ScaleTemperature(temperature, ExpectedTemperatureMin, ExpectedTemperatureMax, MaxScaleCount)
			if !reflect.DeepEqual(first, second) || s1 != s2 {
				t.Errorf("%v != %v", first, second)
			}
		})
	}
}

// fakeConfig records the calls `warn_temperature_scaled` makes.
type fakeConfig struct {
	notified bool
	ensured  []config.ARConfigKeyBase
	sets     int
}

func (f *fakeConfig) EnsureNotificationFlag(key config.ARConfigKeyBase) {
	f.ensured = append(f.ensured, key)
}

func (f *fakeConfig) Get(config.ARConfigKeyBase) any { return f.notified }

func (f *fakeConfig) Set(config.ARConfigKeyBase, any) error {
	f.sets++
	return nil
}

func TestWarnTemperatureScaled(t *testing.T) {
	for _, tc := range []struct {
		name            string
		alreadyNotified bool
		expectWarn      bool
	}{
		{"not_notified_warns", false, true},
		{"already_notified_silent", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
			defer slog.SetDefault(previous)

			cfg := &fakeConfig{notified: tc.alreadyNotified}
			WarnTemperatureScaled(cfg, map[string]any{"key": "val"})

			if len(cfg.ensured) != 1 || cfg.ensured[0] != config.ARConfigKeyNotifiedOptimisticTemperature {
				t.Errorf("ensured %v", cfg.ensured)
			}
			if warned := buf.Len() > 0; warned != tc.expectWarn {
				t.Errorf("warned %v: %q", warned, buf.String())
			}
			if (cfg.sets > 0) != tc.expectWarn {
				t.Errorf("set called %d times", cfg.sets)
			}
		})
	}
}
