package client

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// Real device clientlist (entity-encoded): WG Proton + OVPN, both server 5
const clientlist = "Proton&#62WireGuard&#625&#62&#62&#621&#625&#62&#62&#620&#620&#62Web" +
	"&#60Prtn&#62OpenVPN&#625&#62u&#62p&#621&#626&#62&#62&#620&#620&#62Web"

// mockFetchData answers `fetch_data_callback` with fixed values and
// records the requested sources.
func mockFetchData(requested *[]source.DataSource, values map[source.ID]any) registry.FetchDataMultiFunc {
	return func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
		*requested = append(*requested, srcs...)
		return values, nil
	}
}

// mockRaw answers raw fetches with a fixed body and records the calls.
func mockRaw(calls *[]call, body string) registry.FetchRawFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
		*calls = append(*calls, call{ep, request})
		return &body, nil
	}
}

var clientlistID = source.IDOf(nvram.ARNvramTypeVpncClientlist)

func TestARVpnClientAction(t *testing.T) {
	// Protocol, unit and desired state are stored as given
	a := NewARVpnClientAction(vpn.ARVpnProtocolWireguard, true, 5)
	if a.Protocol != vpn.ARVpnProtocolWireguard || a.Unit != 5 || !a.State {
		t.Errorf("action = %+v", a)
	}

	// `unit` defaults to 1
	if NewARVpnClientAction(vpn.ARVpnProtocolOpenvpn, false, 0).Unit != 1 {
		t.Error("unit default")
	}

	// Actions are equal by protocol, unit and state
	base := action.Repr(NewARVpnClientAction(vpn.ARVpnProtocolWireguard, true, 5))
	if base != action.Repr(NewARVpnClientAction(vpn.ARVpnProtocolWireguard, true, 5)) {
		t.Error("equal actions differ")
	}
	for _, other := range []*ARVpnClientAction{
		NewARVpnClientAction(vpn.ARVpnProtocolWireguard, false, 5),
		NewARVpnClientAction(vpn.ARVpnProtocolWireguard, true, 1),
		NewARVpnClientAction(vpn.ARVpnProtocolOpenvpn, true, 5),
	} {
		if base == action.Repr(other) {
			t.Errorf("%v equals base", action.Repr(other))
		}
	}

	// The action resolves a run_action callable via the registry
	if e := registry.Action(a); e == nil || e.Run == nil {
		t.Error("run_action not registered")
	}
}

func TestRunAction(t *testing.T) {
	ctx := context.Background()
	failed := service.ARServiceResult{Success: false}

	t.Run("fusion rewrites clientlist", func(t *testing.T) {
		// A Fusion device flips the activate flag and runs restart_vpnc
		var requested []source.DataSource
		var calls []call
		c := &registry.Context{
			Read:           mockRead(&calls),
			FetchDataMulti: mockFetchData(&requested, map[source.ID]any{clientlistID: clientlist}),
			FetchRaw:       mockRaw(&calls, "NOT MODIFIED"),
		}
		result, err := runAction(ctx, c, NewARVpnClientAction(vpn.ARVpnProtocolWireguard, false, 5))
		if err != nil || !result.(service.ARServiceResult).Success {
			t.Fatalf("result = %v, %v", result, err)
		}
		if len(requested) != 1 || source.IDOf(requested[0]) != clientlistID {
			t.Errorf("requested = %v", requested)
		}
		push := calls[len(calls)-1]
		if push.ep != endpoint.AREndpointPushData {
			t.Errorf("endpoint = %v", push.ep)
		}
		for _, want := range []string{`"rc_service":"stop_vpnc"`, `"vpnc_unit":0`} {
			if !strings.Contains(push.request, want) {
				t.Errorf("request %s lacks %s", push.request, want)
			}
		}
	})

	t.Run("classic openvpn", func(t *testing.T) {
		// Without a clientlist, OpenVPN uses the per-unit service
		var requested []source.DataSource
		var calls []call
		c := &registry.Context{
			Read:           mockRead(&calls),
			FetchDataMulti: mockFetchData(&requested, map[source.ID]any{}),
			FetchRaw:       mockRaw(&calls, "NOT MODIFIED"),
		}
		result, _ := runAction(ctx, c, NewARVpnClientAction(vpn.ARVpnProtocolOpenvpn, true, 2))
		if !result.(service.ARServiceResult).Success {
			t.Fatal("not successful")
		}
		if r := calls[len(calls)-1].request; !strings.Contains(r, `"rc_service":"start_vpnclient2"`) {
			t.Errorf("request = %s", r)
		}
	})

	t.Run("classic wireguard", func(t *testing.T) {
		// Classic WireGuard flips the enable flag with `start_wgc`
		var requested []source.DataSource
		var calls []call
		c := &registry.Context{
			Read:           mockRead(&calls),
			FetchDataMulti: mockFetchData(&requested, map[source.ID]any{}),
			FetchRaw:       mockRaw(&calls, "NOT MODIFIED"),
		}
		_, _ = runAction(ctx, c, NewARVpnClientAction(vpn.ARVpnProtocolWireguard, true, 3))
		r := calls[len(calls)-1].request
		for _, want := range []string{`"rc_service":"start_wgc 3"`, `"wgc_enable":1`} {
			if !strings.Contains(r, want) {
				t.Errorf("request %s lacks %s", r, want)
			}
		}
	})

	t.Run("unknown client fails", func(t *testing.T) {
		// A Fusion device without the target profile fails without a push
		var requested []source.DataSource
		var calls []call
		c := &registry.Context{
			Read:           mockRead(&calls),
			FetchDataMulti: mockFetchData(&requested, map[source.ID]any{clientlistID: clientlist}),
			FetchRaw:       mockRaw(&calls, ""),
		}
		result, _ := runAction(ctx, c, NewARVpnClientAction(vpn.ARVpnProtocolWireguard, true, 9))
		if result != failed || len(calls) != 0 {
			t.Errorf("result = %v, calls = %d", result, len(calls))
		}
	})

	t.Run("non dict read uses classic", func(t *testing.T) {
		// A failed clientlist read (no values) falls back to the classic backend
		var requested []source.DataSource
		var calls []call
		c := &registry.Context{
			Read:           mockRead(&calls, "NOT MODIFIED"),
			FetchDataMulti: mockFetchData(&requested, nil),
		}
		result, _ := runAction(ctx, c, NewARVpnClientAction(vpn.ARVpnProtocolOpenvpn, true, 1))
		if !result.(service.ARServiceResult).Success {
			t.Fatal("not successful")
		}
		if !strings.Contains(calls[0].request, `"rc_service":"start_vpnclient1"`) {
			t.Errorf("request = %s", calls[0].request)
		}
	})

	t.Run("no data callback fails", func(t *testing.T) {
		// Without a data callback no backend is known; nothing is pushed
		var calls []call
		c := &registry.Context{Read: mockRead(&calls)}
		result, _ := runAction(ctx, c, NewARVpnClientAction(vpn.ARVpnProtocolWireguard, true, 0))
		if result != failed || len(calls) != 0 {
			t.Errorf("result = %v, calls = %d", result, len(calls))
		}
	})

	t.Run("unsupported protocol fails", func(t *testing.T) {
		// A classic protocol without a toggle fails without a push
		var requested []source.DataSource
		var calls []call
		c := &registry.Context{
			Read:           mockRead(&calls),
			FetchDataMulti: mockFetchData(&requested, map[source.ID]any{}),
			FetchRaw:       mockRaw(&calls, ""),
		}
		result, _ := runAction(ctx, c, NewARVpnClientAction(vpn.ARVpnProtocolPptp, true, 0))
		if result != failed || len(calls) != 0 {
			t.Errorf("result = %v, calls = %d", result, len(calls))
		}
	})

	t.Run("falls back to callback", func(t *testing.T) {
		// Without a raw callback the plain callback posts the request
		var requested []source.DataSource
		var calls []call
		c := &registry.Context{
			Read:           mockRead(&calls, "NOT MODIFIED"),
			FetchDataMulti: mockFetchData(&requested, map[source.ID]any{}),
		}
		result, _ := runAction(ctx, c, NewARVpnClientAction(vpn.ARVpnProtocolOpenvpn, false, 1))
		if !result.(service.ARServiceResult).Success {
			t.Fatal("not successful")
		}
		if len(calls) != 1 || !strings.Contains(calls[0].request, `"rc_service":"stop_vpnclient1"`) {
			t.Errorf("calls = %v", calls)
		}
	})
}
