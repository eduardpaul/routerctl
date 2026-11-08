package trafficinterface

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/traffic"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const (
	router = "AA:BB:CC:00:00:01"
	other  = "AA:BB:CC:00:00:02"
)

type metrics_ = map[common.ARMetricType]any
type result_ = map[traffic.ARTrafficLink]map[common.ARMetricType]any

// bandIdentity maps wireless units 0/1/2 to 2g1/5g1/6g1 (via wl{}_nband).
func bandIdentity() *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, 0)): "2",
		source.IDOf(nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, 1)): "1",
		source.IDOf(nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, 2)): "4",
	})
}

func TestSource(t *testing.T) {
	s := NewARTrafficInterfaceSource(router)
	if s.Link() != nil || !reflect.DeepEqual(s.Target(), identifiers.MacAddressFromValueSafe(router)) {
		t.Errorf("link %v target %v", s.Link(), s.Target())
	}
	if source.IDOf(NewARTrafficInterfaceSource(router)) != source.IDOf(NewARTrafficInterfaceSource(router)) {
		t.Error("equal by target")
	}
	if source.IDOf(NewARTrafficInterfaceSource(router)) == source.IDOf(NewARTrafficInterfaceSource(other)) {
		t.Error("different targets")
	}

	// Stash returns the previous sample and stores the new one
	now1 := Counters{"WIRED": {"rx": 1, "tx": 2}}
	now2 := Counters{"WIRED": {"rx": 3, "tx": 4}}
	t1, t2 := time.Unix(1, 0), time.Unix(2, 0)
	if prev, prevTS := s.Stash(now1, t1); prev != nil || prevTS != nil {
		t.Errorf("first stash %v %v", prev, prevTS)
	}
	if prev, prevTS := s.Stash(now2, t2); !reflect.DeepEqual(prev, now1) || !prevTS.Equal(t1) {
		t.Errorf("second stash %v %v", prev, prevTS)
	}
}

func TestFetch(t *testing.T) {
	ctx := context.Background()

	t.Run("modern_used", func(t *testing.T) {
		reads, raws := 0, 0
		c := &registry.Context{
			Read: func(context.Context, endpoint.AREndpoint, string) (any, error) {
				reads++
				return map[string]map[string]int{"WIRED": {"rx": 1, "tx": 2}}, nil
			},
			FetchRaw: func(context.Context, endpoint.AREndpoint, string) (*string, error) {
				raws++
				return nil, nil
			},
		}
		got, _ := fetch(ctx, c)
		if !reflect.DeepEqual(got, Counters{"WIRED": {"rx": 1, "tx": 2}}) || reads != 1 || raws != 0 {
			t.Errorf("got %v, reads %d, raws %d", got, reads, raws)
		}
	})

	// A missing update.cgi falls back to raw appGet, summing dual-WAN
	for _, outcome := range []string{"empty", "404"} {
		t.Run("falls_back_"+outcome, func(t *testing.T) {
			raw := `{"netdev":{"INTERNET_rx":"0x0a","INTERNET_tx":"0x14","INTERNET_rx":"0x01","INTERNET_tx":"0x02"}}`
			c := &registry.Context{
				Read: func(context.Context, endpoint.AREndpoint, string) (any, error) {
					if outcome == "404" {
						return nil, arerror.New404Error("nope")
					}
					return map[string]any{}, nil
				},
				FetchRaw: func(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
					if ep != endpoint.AREndpointFetchData || request != appgetNetdevRequest {
						t.Errorf("raw %s %s", ep, request)
					}
					return &raw, nil
				},
			}
			got, err := fetch(ctx, c)
			if err != nil || !reflect.DeepEqual(got, Counters{"INTERNET": {"rx": 11, "tx": 22}}) {
				t.Errorf("got %v, %v", got, err)
			}
		})
	}

	empty := func(context.Context, endpoint.AREndpoint, string) (any, error) { return map[string]any{}, nil }
	if got, _ := fetch(ctx, &registry.Context{Read: empty}); len(got) != 0 {
		t.Errorf("no_raw_callback: %v", got)
	}
	nilRaw := func(context.Context, endpoint.AREndpoint, string) (*string, error) { return nil, nil }
	if got, _ := fetch(ctx, &registry.Context{Read: empty, FetchRaw: nilRaw}); len(got) != 0 {
		t.Errorf("fallback_non_string: %v", got)
	}
}

func TestFetchStateSamples(t *testing.T) {
	defer func(orig func() time.Time) { now = orig }(now)
	clock := time.Unix(1000, 0)
	now = func() time.Time { return clock }

	s := NewARTrafficInterfaceSource(nil)
	c := &registry.Context{Read: func(context.Context, endpoint.AREndpoint, string) (any, error) {
		return map[string]map[string]int{"WIRED": {"rx": 1, "tx": 2}}, nil
	}}

	first, _ := fetchState(context.Background(), c, s)
	fm := first.(map[string]any)
	if !reflect.DeepEqual(fm["now"], Counters{"WIRED": {"rx": 1, "tx": 2}}) || fm["prev"].(Counters) != nil || fm["delta_time"].(*float64) != nil {
		t.Errorf("first %v", fm)
	}

	clock = clock.Add(2500 * time.Millisecond)
	second, _ := fetchState(context.Background(), c, s)
	sm := second.(map[string]any)
	if !reflect.DeepEqual(sm["prev"], Counters{"WIRED": {"rx": 1, "tx": 2}}) || *sm["delta_time"].(*float64) != 2.5 {
		t.Errorf("second %v", sm)
	}
}

func dt(v float64) *float64 { return &v }

func TestTranslateState(t *testing.T) {
	if got := translateState(nil, nil).(result_); len(got) != 0 {
		t.Errorf("non_dict: %v", got)
	}

	tests := []struct {
		name     string
		data     map[string]any
		identity *device.ARDeviceIdentity
		want     result_
	}{
		{
			"zero_speeds_without_history",
			map[string]any{"now": Counters{"WIRED": {"rx": 100, "tx": 200}}, "prev": Counters(nil)},
			nil,
			result_{traffic.ARTrafficTypeWired: {common.ARMetricTypeRx: 100, common.ARMetricTypeTx: 200, common.ARMetricTypeRxSpeed: 0.0, common.ARMetricTypeTxSpeed: 0.0}},
		},
		{
			"overflow_zeroes_speed",
			map[string]any{
				"now":        Counters{"WIRED": {"rx": 5, "tx": 200}},
				"prev":       Counters{"WIRED": {"rx": 100, "tx": 40}},
				"delta_time": dt(2.0),
			},
			nil,
			result_{traffic.ARTrafficTypeWired: {common.ARMetricTypeRx: 5, common.ARMetricTypeTx: 200, common.ARMetricTypeRxSpeed: 0.0, common.ARMetricTypeTxSpeed: 640.0}},
		},
		{
			"wireless_skipped_without_identity",
			map[string]any{"now": Counters{"WIRELESS0": {"rx": 1, "tx": 2}}},
			nil,
			result_{},
		},
		{
			"partial_counters",
			map[string]any{"now": Counters{"WIRED": {"rx": 5}}},
			nil,
			result_{traffic.ARTrafficTypeWired: {common.ARMetricTypeRx: 5, common.ARMetricTypeRxSpeed: 0.0}},
		},
		{
			"unknown_iface_and_missing_prev_kind",
			map[string]any{
				"now":        Counters{"FOO": {"rx": 1, "tx": 2}, "WIRED": {"rx": 100, "tx": 200}},
				"prev":       Counters{"WIRED": {"rx": 20}},
				"delta_time": dt(2.0),
			},
			nil,
			result_{traffic.ARTrafficTypeWired: {common.ARMetricTypeRx: 100, common.ARMetricTypeTx: 200, common.ARMetricTypeRxSpeed: 320.0, common.ARMetricTypeTxSpeed: 0.0}},
		},
	}
	for _, delta := range []*float64{dt(0.0), dt(0.5), nil} {
		tests = append(tests, struct {
			name     string
			data     map[string]any
			identity *device.ARDeviceIdentity
			want     result_
		}{
			"short_interval",
			map[string]any{
				"now":        Counters{"WIRED": {"rx": 5, "tx": 6}},
				"prev":       Counters{"WIRED": {"rx": 1, "tx": 2}},
				"delta_time": delta,
			},
			nil,
			result_{traffic.ARTrafficTypeWired: {common.ARMetricTypeRx: 5, common.ARMetricTypeTx: 6, common.ARMetricTypeRxSpeed: 0.0, common.ARMetricTypeTxSpeed: 0.0}},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translateState(tt.data, &registry.TranslateContext{Identity: tt.identity})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTranslateStateFullMap(t *testing.T) {
	data := map[string]any{
		"now": Counters{
			"WIRED":     {"rx": 100, "tx": 200},
			"INTERNET":  {"rx": 50, "tx": 60},
			"INTERNET1": {"rx": 8, "tx": 8},
			"BRIDGE":    {"rx": 4, "tx": 4},
			"WIRELESS0": {"rx": 10, "tx": 20},
			"LACP1":     {"rx": 5, "tx": 7},
			"LACP2":     {"rx": 3, "tx": 4},
			"WIRELESS9": {"rx": 1, "tx": 1},
		},
		"prev": Counters{
			"WIRED":     {"rx": 20, "tx": 40},
			"INTERNET":  {"rx": 10, "tx": 12},
			"INTERNET1": {"rx": 0, "tx": 0},
			"BRIDGE":    {"rx": 0, "tx": 0},
			"WIRELESS0": {"rx": 2, "tx": 4},
			"LACP1":     {"rx": 1, "tx": 1},
			"LACP2":     {"rx": 1, "tx": 1},
		},
		"delta_time": dt(2.0),
	}

	result := translateState(data, &registry.TranslateContext{Identity: bandIdentity()}).(result_)

	m := func(rx, tx int, rxs, txs float64) metrics_ {
		return metrics_{common.ARMetricTypeRx: rx, common.ARMetricTypeTx: tx, common.ARMetricTypeRxSpeed: rxs, common.ARMetricTypeTxSpeed: txs}
	}
	want := map[traffic.ARTrafficLink]metrics_{
		traffic.ARTrafficTypeWired: m(100, 200, 320, 640),
		traffic.ARTrafficTypeWan:   m(50, 60, 160, 192),
		traffic.ARTrafficTypeUsb:   m(8, 8, 32, 32),
		wifi.ARWiFiBandBand2g1:     m(10, 20, 32, 64),
		traffic.ARTrafficTypeLacp1: m(5, 7, 16, 24),
		// Cumulative LACP = LACP1 + LACP2
		traffic.ARTrafficTypeLacp: m(8, 11, 24, 36),
	}
	for link, metrics := range want {
		if !reflect.DeepEqual(result[link], metrics) {
			t.Errorf("%v: got %v, want %v", link, result[link], metrics)
		}
	}
	// Unknown wireless unit (no band) is skipped
	if _, ok := result[wifi.ARWiFiBandUnknown]; ok {
		t.Error("unknown band present")
	}
}

func TestRegistered(t *testing.T) {
	entry := registry.SourceByName("ARTrafficInterfaceSource")
	if entry == nil || entry.Fetch == nil || entry.Translate == nil {
		t.Fatal("not registered")
	}
}
