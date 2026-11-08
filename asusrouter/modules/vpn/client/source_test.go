package client

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

type call struct {
	ep      endpoint.AREndpoint
	request string
}

// mockRead answers reads in order and records the calls.
func mockRead(calls *[]call, replies ...any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		if len(*calls) <= len(replies) {
			return replies[len(*calls)-1], nil
		}
		return map[string]any{}, nil
	}
}

func TestFetchState(t *testing.T) {
	t.Run("fusion skips vpn cgi", func(t *testing.T) {
		// When a clientlist is present, the classic vpn.cgi is not fetched
		var calls []call
		c := &registry.Context{Read: mockRead(&calls, map[string]any{"vpnc_clientlist": "x"})}
		if _, err := fetchState(context.Background(), c, ARVpnClientSourceUniversal); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 1 || calls[0].ep != endpoint.AREndpointFetchData {
			t.Fatalf("calls = %v", calls)
		}
		for _, want := range []string{"get_vpnc_status()", "vpnc_clientlist", "vpn_client1_state"} {
			if !strings.Contains(calls[0].request, want) {
				t.Errorf("request lacks %s", want)
			}
		}
		// dict.fromkeys: shared wgc keys appear once
		if n := strings.Count(calls[0].request, "nvram_get(wgc1_priv)"); n != 1 {
			t.Errorf("wgc1_priv requested %d times", n)
		}
	})

	t.Run("classic fetches vpn cgi", func(t *testing.T) {
		// Without a clientlist, the classic vpn.cgi status is fetched too
		var calls []call
		c := &registry.Context{Read: mockRead(&calls, map[string]any{}, map[string]any{"vpn_client1_status": "None"})}
		data, _ := fetchState(context.Background(), c, ARVpnClientSourceUniversal)
		if len(calls) != 2 || calls[1].ep != endpoint.AREndpointFetchVpnStatus {
			t.Fatalf("calls = %v", calls)
		}
		if _, ok := data.(map[string]any)["_vpn_status"]; !ok {
			t.Error("status not stashed")
		}
	})
}

func TestTranslateState(t *testing.T) {
	type result = map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnClientField]any

	// Non-dict raw data yields an empty result
	if got := translateState(nil, nil).(result); len(got) != 0 {
		t.Errorf("translateState(nil) = %v", got)
	}

	// A clientlist routes to the Fusion backend
	got := translateState(map[string]any{"vpnc_clientlist": "P>OpenVPN>3>u>p>1>6>>>0>0>Web"}, nil).(result)
	if len(got) != 1 || got[vpn.ARVpnProtocolOpenvpn] == nil {
		t.Errorf("fusion = %v", got)
	}

	// No clientlist routes to the classic per-unit backend
	got = translateState(map[string]any{"vpn_client1_state": "2", "vpn_client1_desc": "C"}, nil).(result)
	if len(got) != 1 || got[vpn.ARVpnProtocolOpenvpn][1][vpn.ARVpnClientFieldName] != "C" {
		t.Errorf("classic = %v", got)
	}
}

func TestRegistration(t *testing.T) {
	e := registry.Source(&ARVpnClientSource{})
	if e == nil || reflect.ValueOf(e.Fetch).Pointer() != reflect.ValueOf(fetchState).Pointer() {
		t.Error("fetch_state not registered")
	}
}
