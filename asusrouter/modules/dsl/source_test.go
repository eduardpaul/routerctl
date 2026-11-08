package dsl

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func testIdentity(dslSupport bool) *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(support.ARSupportSourceUniversal): map[support.ARSupportType]any{
			support.ARSupportTypeDsl: dslSupport,
		},
	})
}

func TestReadRate(t *testing.T) {
	cases := []struct {
		value    any
		expected float64
	}{
		{"12345 Kbps", 12345000.0},
		{"12345", 12345000.0},
		{12345, 12345000.0},
		{"0 Kbps", 0.0},
	}
	for _, c := range cases {
		if got := readRate(c.value); got == nil || *got != c.expected {
			t.Errorf("readRate(%v) = %v, want %v", c.value, got, c.expected)
		}
	}
	for _, value := range []any{nil, "", "N/A", "-- Kbps"} {
		if got := readRate(value); got != nil {
			t.Errorf("readRate(%v) = %v, want nil", value, *got)
		}
	}
}

func TestARDSLSource(t *testing.T) {
	if source.IDOf(&ARDSLSource{}) != source.IDOf(&ARDSLSource{}) {
		t.Error("all DSL sources should be equal")
	}
	if got := source.Repr(&ARDSLSource{}); got != "<ARDSLSource>" {
		t.Errorf("repr = %q", got)
	}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()
	values := map[source.ID]any{source.IDOf(nvram.ARNvramTypeDslDatarateDown): "1 Kbps"}

	t.Run("fetches when supported", func(t *testing.T) {
		var requested []source.DataSource
		read := false
		c := &registry.Context{
			Identity: testIdentity(true),
			Read: func(context.Context, endpoint.AREndpoint, string) (any, error) {
				read = true
				return nil, nil
			},
			FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
				requested = srcs
				return values, nil
			},
		}
		got, err := fetchState(ctx, c, &ARDSLSource{})
		if err != nil || !reflect.DeepEqual(got, values) {
			t.Errorf("got %v, %v", got, err)
		}
		if read {
			t.Error("callback should not be awaited")
		}
		want := []source.DataSource{nvram.ARNvramTypeDslDatarateDown, nvram.ARNvramTypeDslDatarateUp}
		if !reflect.DeepEqual(requested, want) {
			t.Errorf("requested %v", requested)
		}
	})

	t.Run("nil response", func(t *testing.T) {
		c := &registry.Context{
			Identity: testIdentity(true),
			FetchDataMulti: func(context.Context, []source.DataSource, registry.Kwargs) (map[source.ID]any, error) {
				return nil, nil
			},
		}
		got, err := fetchState(ctx, c, &ARDSLSource{})
		if err != nil || len(got.(map[source.ID]any)) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})

	t.Run("no data callback", func(t *testing.T) {
		got, err := fetchState(ctx, &registry.Context{Identity: testIdentity(true)}, &ARDSLSource{})
		if err != nil || len(got.(map[source.ID]any)) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})

	for name, identity := range map[string]*device.ARDeviceIdentity{
		"no identity": nil,
		"unsupported": testIdentity(false),
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			c := &registry.Context{
				Identity: identity,
				FetchDataMulti: func(context.Context, []source.DataSource, registry.Kwargs) (map[source.ID]any, error) {
					called = true
					return values, nil
				},
			}
			got, err := fetchState(ctx, c, &ARDSLSource{})
			if err != nil || len(got.(map[source.ID]any)) != 0 || called {
				t.Errorf("got %v, %v, called=%v", got, err, called)
			}
		})
	}
}

func TestTranslateState(t *testing.T) {
	down := source.IDOf(nvram.ARNvramTypeDslDatarateDown)
	up := source.IDOf(nvram.ARNvramTypeDslDatarateUp)

	got := translateState(map[source.ID]any{down: "12345 Kbps", up: "678 Kbps"}, nil)
	want := map[common.ARMetricType]any{
		common.ARMetricTypeDownloadSpeed: 12345000.0,
		common.ARMetricTypeUploadSpeed:   678000.0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("both directions: %v", got)
	}

	got = translateState(map[source.ID]any{down: "5 Kbps"}, nil)
	if !reflect.DeepEqual(got, map[common.ARMetricType]any{common.ARMetricTypeDownloadSpeed: 5000.0}) {
		t.Errorf("missing fields: %v", got)
	}

	for _, data := range []any{nil, "x", 42, map[source.ID]any{}} {
		if got := translateState(data, nil).(map[common.ARMetricType]any); len(got) != 0 {
			t.Errorf("translateState(%v) = %v", data, got)
		}
	}
}

func TestRegistersCallable(t *testing.T) {
	e := registry.SourceByName("ARDSLSource")
	if e == nil || e.Fetch == nil || e.Translate == nil {
		t.Error("ARDSLSource is not registered")
	}
}
