package server

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/server/openvpn"
	"github.com/eduardpaul/routerctl/asusrouter/parity/canonical"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// canon renders a value in its canonical JSON form for comparisons.
func canon(v any) any {
	var out any
	b, _ := json.Marshal(canonical.Of(v))
	_ = json.Unmarshal(b, &out)
	return out
}

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
	t.Run("fetches appget", func(t *testing.T) {
		// The request carries the WG hook and nvram keys
		var calls []call
		c := &registry.Context{Read: mockRead(&calls, map[string]any{"VPNServer_enable": "0"})}
		if _, err := fetchState(context.Background(), c, ARVpnServerSourceUniversal); err != nil {
			t.Fatal(err)
		}
		if calls[0].ep != endpoint.AREndpointFetchData {
			t.Errorf("endpoint = %v", calls[0].ep)
		}
		if !strings.HasPrefix(calls[0].request, "hook=get_wgsc_status();") {
			t.Errorf("request = %s", calls[0].request)
		}
		if !strings.Contains(calls[0].request, "nvram_get(wgs_enable)") {
			t.Errorf("request = %s", calls[0].request)
		}
	})

	t.Run("ovpn disabled single call", func(t *testing.T) {
		// A disabled OpenVPN server skips the extra status fetch
		var calls []call
		c := &registry.Context{Read: mockRead(&calls, map[string]any{"VPNServer_enable": "0"})}
		_, _ = fetchState(context.Background(), c, ARVpnServerSourceUniversal)
		if len(calls) != 1 {
			t.Errorf("calls = %d", len(calls))
		}
	})

	t.Run("ovpn enabled second call", func(t *testing.T) {
		// An enabled OpenVPN server fetches the connected-client status
		var calls []call
		c := &registry.Context{Read: mockRead(&calls,
			map[string]any{"VPNServer_enable": "1"},
			// The status endpoint returns raw text; the source parses it
			"<vpnserver>\n1.2.3.4:5 10.0.0.2 alice\n</vpnserver>",
		)}
		result, _ := fetchState(context.Background(), c, ARVpnServerSourceUniversal)
		if len(calls) != 2 {
			t.Fatalf("calls = %d", len(calls))
		}
		if calls[1].ep != endpoint.AREndpointFetchVpnOpenvpnStatus {
			t.Errorf("endpoint = %v", calls[1].ep)
		}
		got := canon(result.(map[string]any)[openvpn.ClientStatusKey])
		want := canon(map[string]any{"connected": []any{
			map[string]any{"name": "alice", "vpn_ip": "10.0.0.2", "remote": "1.2.3.4:5"},
		}})
		if !reflect.DeepEqual(got, want) {
			t.Errorf("status = %v", got)
		}
	})

	t.Run("non dict response", func(t *testing.T) {
		// A non-dict response is returned untouched, no extra call
		var calls []call
		c := &registry.Context{Read: mockRead(&calls, nil)}
		result, _ := fetchState(context.Background(), c, ARVpnServerSourceUniversal)
		if result != nil || len(calls) != 1 {
			t.Errorf("result = %v, calls = %d", result, len(calls))
		}
	})
}

func TestTranslateState(t *testing.T) {
	// Non-dict data yields an empty result
	for _, d := range []any{nil, "text", 42, []any{}} {
		if got := translateState(d, nil).(map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnServerField]any); len(got) != 0 {
			t.Errorf("translateState(%v) = %v", d, got)
		}
	}

	// Both backends contribute, grouped by protocol
	result := translateState(map[string]any{
		"wgs_enable":        "1",
		"VPNServer_enable":  "1",
		"vpn_server_unit":   "1",
		"vpn_server1_state": "2",
	}, nil).(map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnServerField]any)
	if len(result) != 2 || result[vpn.ARVpnProtocolWireguard] == nil || result[vpn.ARVpnProtocolOpenvpn] == nil {
		t.Errorf("protocols = %v", result)
	}

	// A protocol with no data is omitted
	result = translateState(map[string]any{"wgs_enable": "1"}, nil).(map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnServerField]any)
	if len(result) != 1 || result[vpn.ARVpnProtocolWireguard][1][vpn.ARVpnServerFieldEnabled] != true {
		t.Errorf("result = %v", result)
	}
}

func TestSourceIsRegistered(t *testing.T) {
	e := registry.Source(ARVpnServerSourceUniversal)
	if e == nil {
		t.Fatal("not registered")
	}
	if reflect.ValueOf(e.Fetch).Pointer() != reflect.ValueOf(fetchState).Pointer() {
		t.Error("fetch_state not registered")
	}
	if reflect.ValueOf(e.Translate).Pointer() != reflect.ValueOf(translateState).Pointer() {
		t.Error("translate_state not registered")
	}
}
