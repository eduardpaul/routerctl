package server

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// modernIdentity builds an identity on modern stock firmware (388).
func modernIdentity() *device.ARDeviceIdentity {
	// Python sets `identity._firmware`; the Go identity is built from the
	// raw nvram firmware values instead
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(nvram.ARNvramTypeFwMajor): "3.0.0.4",
		source.IDOf(nvram.ARNvramTypeFwMinor): "388",
		source.IDOf(nvram.ARNvramTypeFwBuild): "0",
	})
}

// mockRaw answers raw fetches with a fixed body and records the calls.
func mockRaw(calls *[]call, body string) registry.FetchRawFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
		*calls = append(*calls, call{ep, request})
		return &body, nil
	}
}

func TestARVpnServerAction(t *testing.T) {
	// The protocol, unit and desired state are stored as given
	a := NewARVpnServerAction(vpn.ARVpnProtocolWireguard, true, 0)
	if a.Protocol != vpn.ARVpnProtocolWireguard || a.Unit != 1 || !a.State {
		t.Errorf("action = %+v", a)
	}

	// Actions are equal by protocol, unit and state
	id := func(x action.ARAction) source.ID { return source.ID(action.Repr(x)) }
	base := id(NewARVpnServerAction(vpn.ARVpnProtocolWireguard, true, 0))
	if base != id(NewARVpnServerAction(vpn.ARVpnProtocolWireguard, true, 0)) {
		t.Error("equal actions differ")
	}
	for _, other := range []*ARVpnServerAction{
		NewARVpnServerAction(vpn.ARVpnProtocolWireguard, false, 0),
		NewARVpnServerAction(vpn.ARVpnProtocolWireguard, true, 2),
		NewARVpnServerAction(vpn.ARVpnProtocolOpenvpn, true, 0),
	} {
		if base == id(other) {
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

	t.Run("wireguard pushes", func(t *testing.T) {
		// WireGuard toggles the server and restarts dnsmasq
		var calls []call
		c := &registry.Context{Read: mockRead(&calls), FetchRaw: mockRaw(&calls, "NOT MODIFIED")}
		result, err := runAction(ctx, c, NewARVpnServerAction(vpn.ARVpnProtocolWireguard, true, 0))
		if err != nil || !result.(service.ARServiceResult).Success {
			t.Fatalf("result = %v, %v", result, err)
		}
		push := calls[len(calls)-1]
		if push.ep != endpoint.AREndpointPushData {
			t.Errorf("endpoint = %v", push.ep)
		}
		for _, want := range []string{`"rc_service":"restart_wgs;restart_dnsmasq"`, `"wgs_enable":1`, `"wgs_unit":1`} {
			if !strings.Contains(push.request, want) {
				t.Errorf("request %s lacks %s", push.request, want)
			}
		}
	})

	t.Run("openvpn modern pushes", func(t *testing.T) {
		// Modern OpenVPN toggles the enable flag with a service chain
		var calls []call
		c := &registry.Context{
			Read:     mockRead(&calls),
			FetchRaw: mockRaw(&calls, "NOT MODIFIED"),
			Identity: modernIdentity(),
		}
		result, _ := runAction(ctx, c, NewARVpnServerAction(vpn.ARVpnProtocolOpenvpn, true, 0))
		if !result.(service.ARServiceResult).Success {
			t.Fatal("not successful")
		}
		request := calls[len(calls)-1].request
		for _, want := range []string{
			`"rc_service":"restart_openvpnd;restart_chpass;restart_samba;restart_dnsmasq"`,
			`"VPNServer_enable":1`,
		} {
			if !strings.Contains(request, want) {
				t.Errorf("request %s lacks %s", request, want)
			}
		}
	})

	t.Run("openvpn legacy pushes", func(t *testing.T) {
		// Without an identity OpenVPN uses the per-unit start service
		var calls []call
		c := &registry.Context{Read: mockRead(&calls), FetchRaw: mockRaw(&calls, "NOT MODIFIED")}
		result, _ := runAction(ctx, c, NewARVpnServerAction(vpn.ARVpnProtocolOpenvpn, true, 2))
		if !result.(service.ARServiceResult).Success {
			t.Fatal("not successful")
		}
		if request := calls[len(calls)-1].request; !strings.Contains(request, `"rc_service":"start_vpnserver2"`) {
			t.Errorf("request = %s", request)
		}
	})

	t.Run("unsupported protocol fails", func(t *testing.T) {
		// A protocol without a backend fails without a push
		var calls []call
		c := &registry.Context{Read: mockRead(&calls), FetchRaw: mockRaw(&calls, "")}
		result, _ := runAction(ctx, c, NewARVpnServerAction(vpn.ARVpnProtocolIpsec, true, 0))
		if result != (service.ARServiceResult{Success: false}) || len(calls) != 0 {
			t.Errorf("result = %v, calls = %d", result, len(calls))
		}
	})

	t.Run("falls back to callback", func(t *testing.T) {
		// Without a raw callback the plain callback posts the request
		var calls []call
		c := &registry.Context{Read: mockRead(&calls, "NOT MODIFIED")}
		result, _ := runAction(ctx, c, NewARVpnServerAction(vpn.ARVpnProtocolWireguard, false, 0))
		if !result.(service.ARServiceResult).Success {
			t.Fatal("not successful")
		}
		if calls[0].ep != endpoint.AREndpointPushData || !strings.Contains(calls[0].request, `"wgs_enable":0`) {
			t.Errorf("call = %+v", calls[0])
		}
	})
}
