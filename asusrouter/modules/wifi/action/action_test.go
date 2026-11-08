package action

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// identity builds an identity with 2g1 on unit 0 and 5g1 on unit 1
// (Python uses a SimpleNamespace stub), from the per-radio nband codes.
func identity() *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, 0)): "2",
		source.IDOf(nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, 1)): "1",
	})
}

func TestARWiFiAction(t *testing.T) {
	t.Run("StoresBandAndState", func(t *testing.T) {
		a := NewARWiFiAction(wifi.ARWiFiBandBand5g1, true)
		if a.Band != wifi.ARWiFiBandBand5g1 || a.State != true {
			t.Errorf("action = %+v", a)
		}
	})
	t.Run("EqualityAndHash", func(t *testing.T) {
		// Go actions are equal by name and key (Python __eq__/__hash__)
		key := func(a *ARWiFiAction) []any { return append([]any{a.ActionName()}, a.ActionKey()...) }
		base := key(NewARWiFiAction(wifi.ARWiFiBandBand2g1, true))
		if !reflect.DeepEqual(base, key(NewARWiFiAction(wifi.ARWiFiBandBand2g1, true))) {
			t.Error("equal actions differ")
		}
		if reflect.DeepEqual(base, key(NewARWiFiAction(wifi.ARWiFiBandBand2g1, false))) {
			t.Error("state ignored")
		}
		if reflect.DeepEqual(base, key(NewARWiFiAction(wifi.ARWiFiBandBand5g1, true))) {
			t.Error("band ignored")
		}
	})
	t.Run("Registered", func(t *testing.T) {
		e := registry.Action(NewARWiFiAction(wifi.ARWiFiBandBand2g1, true))
		if e == nil || e.Run == nil {
			t.Error("ARWiFiAction must register run_action")
		}
	})
}

type call struct {
	ep      endpoint.AREndpoint
	request string
}

func TestRunAction(t *testing.T) {
	ctx := context.Background()
	t.Run("EnableViaRawCallback", func(t *testing.T) {
		var rawCalls, readCalls []call
		c := &registry.Context{
			FetchRaw: func(ctx context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
				rawCalls = append(rawCalls, call{ep, request})
				body := "NOT MODIFIED"
				return &body, nil
			},
			Read: func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
				readCalls = append(readCalls, call{ep, request})
				return nil, nil
			},
			Identity: identity(),
		}
		result, err := runAction(ctx, c, NewARWiFiAction(wifi.ARWiFiBandBand5g1, true))
		if err != nil {
			t.Fatal(err)
		}
		if r, ok := result.(service.ARServiceResult); !ok || !r.Success {
			t.Errorf("result = %#v, want success", result)
		}
		if len(readCalls) != 0 {
			t.Errorf("read called: %v", readCalls)
		}
		if len(rawCalls) != 1 {
			t.Fatalf("raw calls = %v", rawCalls)
		}
		got := rawCalls[0]
		if got.ep != endpoint.AREndpointPushData ||
			!strings.Contains(got.request, `"rc_service":"restart_wireless"`) ||
			!strings.Contains(got.request, `"wl1_radio":1`) {
			t.Errorf("call = %+v", got)
		}
	})
	t.Run("DisableViaCallback", func(t *testing.T) {
		var calls []call
		c := &registry.Context{
			Read: func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
				calls = append(calls, call{ep, request})
				return map[string]any{"run_service": "restart_wireless"}, nil
			},
			Identity: identity(),
		}
		result, err := runAction(ctx, c, NewARWiFiAction(wifi.ARWiFiBandBand2g1, false))
		if err != nil {
			t.Fatal(err)
		}
		if r, ok := result.(service.ARServiceResult); !ok || !r.Success {
			t.Errorf("result = %#v, want success", result)
		}
		if len(calls) != 1 || calls[0].ep != endpoint.AREndpointPushData || !strings.Contains(calls[0].request, `"wl0_radio":0`) {
			t.Errorf("calls = %v", calls)
		}
	})
	for _, tt := range []struct {
		name     string
		identity *device.ARDeviceIdentity
		band     wifi.ARWiFiBand
	}{
		{"BandNotInWifi", identity(), wifi.ARWiFiBandBand6g1},
		{"NoIdentity", nil, wifi.ARWiFiBandBand2g1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			c := &registry.Context{
				Read: func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
					calls = append(calls, call{ep, request})
					return nil, nil
				},
			}
			if tt.identity != nil {
				c.Identity = tt.identity
			}
			result, err := runAction(ctx, c, NewARWiFiAction(tt.band, true))
			if err != nil {
				t.Fatal(err)
			}
			if result != (service.ARServiceResult{Success: false}) {
				t.Errorf("result = %#v, want failure", result)
			}
			if len(calls) != 0 {
				t.Errorf("calls = %v, want none", calls)
			}
		})
	}
}
