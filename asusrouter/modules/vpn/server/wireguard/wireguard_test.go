package wireguard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/parity/canonical"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// canon renders a value in its canonical JSON form for comparisons.
func canon(v any) any {
	var out any
	b, _ := json.Marshal(canonical.Of(v))
	_ = json.Unmarshal(b, &out)
	return out
}

func assertCanon(t *testing.T, got, want any) {
	t.Helper()
	if g, w := canon(got), canon(want); !reflect.DeepEqual(g, w) {
		gb, _ := json.Marshal(g)
		wb, _ := json.Marshal(w)
		t.Errorf("got %s, want %s", gb, wb)
	}
}

// Server settings + one peer, mirroring the real (encoded) nvram shape
func data() map[string]any {
	return map[string]any{
		"wgs_enable":     "1",
		"wgs_addr":       "10.55.0.1/24",
		"wgs_port":       "443",
		"wgs_dns":        "1",
		"wgs_nat6":       "1",
		"wgs_psk":        "1",
		"wgs_alive":      "25",
		"wgs_lanaccess":  "1",
		"wgs_pub":        "PUBKEYVALUE=",
		"wgs_priv":       "PRIVKEYVALUE=",
		"wgs1_c1_enable": "1",
		"wgs1_c1_name":   "My%2FPeer",
		"wgs1_c1_addr":   "10%2E55%2E0%2E24%2F32",
		"wgs1_c1_aips":   "10%2E55%2E0%2E24%2F32",
		"wgs1_c1_caips":  "0%2E0%2E0%2E0%2F0",
		string(Hook): map[string]any{
			"client_status": []any{map[string]any{"index": int64(1), "status": int64(1)}},
		},
	}
}

func TestDecode(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  *string
	}{
		{"My%2FPeer", strp("My/Peer")},
		{"plain", strp("plain")},
		{nil, nil},
		{"", nil},
	} {
		got := decode(tc.value)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("decode(%v) = %v", tc.value, got)
		}
	}
}

func strp(s string) *string { return &s }

func TestUnquote(t *testing.T) {
	for in, want := range map[string]string{
		"a%20b":     "a b",
		"%zz":       "%zz",
		"100%":      "100%",
		"%E2%82%AC": "€",
		"é%41":      "éA",
	} {
		if got := unquote(in); got != want {
			t.Errorf("unquote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodeInterfaces(t *testing.T) {
	result := decodeInterfaces("10%2E0%2E0%2E1%2F32")
	if len(result) != 1 || result[0].String() != "10.0.0.1/32" {
		t.Errorf("decodeInterfaces = %v", result)
	}
	if got := decodeInterfaces(nil); len(got) != 0 {
		t.Errorf("decodeInterfaces(nil) = %v", got)
	}
}

func TestNvramItems(t *testing.T) {
	keys := map[string]bool{}
	for _, item := range NvramItems() {
		keys[item.AsHook().Args] = true
	}
	for _, want := range []string{"wgs_enable", "wgs_priv", "wgs1_c1_name", fmt.Sprintf("wgs1_c%d_caips", MaxPeers)} {
		if !keys[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestPeerStatus(t *testing.T) {
	// A missing/invalid hook yields no status
	if got := peerStatus(map[string]any{}); len(got) != 0 {
		t.Errorf("peerStatus({}) = %v", got)
	}

	// Status 1 -> CONNECTED, otherwise DISCONNECTED; bad index skipped
	got := peerStatus(map[string]any{
		string(Hook): map[string]any{
			"client_status": []any{
				map[string]any{"index": int64(1), "status": int64(1)},
				map[string]any{"index": int64(2), "status": int64(0)},
				map[string]any{"index": nil, "status": int64(1)},
			},
		},
	})
	want := map[int]vpn.ARVpnState{1: vpn.ARVpnStateConnected, 2: vpn.ARVpnStateDisconnected}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("peerStatus = %v", got)
	}
}

func TestPeer(t *testing.T) {
	// An empty slot returns nil
	if got := peerEntry(map[string]any{}, 5, nil); got != nil {
		t.Errorf("empty slot = %v", got)
	}

	// A peer with no live status omits STATE
	p := peerEntry(data(), 1, map[int]vpn.ARVpnState{})
	if p == nil {
		t.Fatal("peer is nil")
	}
	if _, ok := p[vpn.ARVpnPeerFieldState]; ok {
		t.Error("peer has a state")
	}

	// A peer with live status carries STATE
	p = peerEntry(data(), 1, map[int]vpn.ARVpnState{1: vpn.ARVpnStateConnected})
	if p[vpn.ARVpnPeerFieldState] != vpn.ARVpnStateConnected {
		t.Errorf("state = %v", p[vpn.ARVpnPeerFieldState])
	}
}

func TestTranslate(t *testing.T) {
	// No server settings yields an empty result
	if got := Translate(map[string]any{}); len(got) != 0 {
		t.Errorf("Translate({}) = %v", got)
	}

	// A full payload maps server fields, keys, and the decoded peer
	server := Translate(data())[Unit]
	if _, ok := server[vpn.ARVpnServerFieldPrivateKey].(identifiers.Password); !ok {
		t.Errorf("private key is %T", server[vpn.ARVpnServerFieldPrivateKey])
	}
	assertCanon(t, server, map[string]any{
		"enabled":     true,
		"address":     "10.55.0.1/24",
		"port":        443,
		"allow_dns":   true,
		"nat6":        true,
		"psk":         true,
		"keepalive":   25,
		"lan_access":  true,
		"public_key":  "PUBKEYVALUE=",
		"private_key": "PRIVKEYVALUE=",
		"clients": []any{map[string]any{
			"enabled":            true,
			"name":               "My/Peer",
			"address":            []any{"10.55.0.24/32"},
			"allowed_ips":        []any{"10.55.0.24/32"},
			"client_allowed_ips": []any{"0.0.0.0/0"},
			"state":              2,
		}},
	})

	// Server without peers omits the CLIENTS key
	server = Translate(map[string]any{"wgs_enable": "1"})[Unit]
	if _, ok := server[vpn.ARVpnServerFieldClients]; ok {
		t.Error("server has clients")
	}
}

func TestBuildTogglePayload(t *testing.T) {
	wantServices := []service.ARServiceInput{
		common.ARServiceWireguardServerRestart,
		common.ARServiceDnsRestart,
	}
	for _, tc := range []struct {
		unit      int
		state     bool
		arguments [][2]any
	}{
		// Enabling restarts the server and dnsmasq, flags nvram on
		{1, true, [][2]any{{"wgs_enable", 1}, {"wgs_unit", 1}, {"id", 1}}},
		// Disabling uses the same services and flags nvram off
		{2, false, [][2]any{{"wgs_enable", 0}, {"wgs_unit", 2}, {"id", 2}}},
	} {
		services, arguments := BuildTogglePayload(tc.unit, tc.state)
		if !reflect.DeepEqual(services, wantServices) {
			t.Errorf("services = %v", services)
		}
		if !reflect.DeepEqual(arguments, tc.arguments) {
			t.Errorf("arguments = %v", arguments)
		}
	}
}
