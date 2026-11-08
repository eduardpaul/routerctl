package classic

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/parity/canonical"
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

const fullStatus = "REMOTE,198.51.100.7:1194,udp>Updated,2024-01-02 03:04:05" +
	">TUN/TAP read bytes,111>TUN/TAP write bytes,222" +
	">TCP/UDP read bytes,12345>TCP/UDP write bytes,678" +
	">Auth read bytes,333>pre-compress bytes,44>post-compress bytes,55" +
	">pre-decompress bytes,66>post-decompress bytes,77>END"

// One active OpenVPN client (unit 1) and one WireGuard client (unit 2)
func data() map[string]any {
	return map[string]any{
		"vpn_clientx_eas":      "1,",
		"vpn_client1_state":    "2",
		"vpn_client1_desc":     "MyOVPN",
		"vpn_client1_addr":     "vpn.host.test",
		"vpn_client1_port":     "1194",
		"vpn_client1_username": "bob",
		"wgc2_enable":          "1",
		"wgc2_addr":            "10.6.0.2/32",
		"wgc2_ppub":            "cHViZmFrZQ==",
		"wgc2_ep_addr":         "203.0.113.9",
		"wgc2_ep_port":         "51820",
		StatusKey: map[string]any{
			"vpn_client1_status": fullStatus,
			"vpn_client1_ip":     "10.8.0.6",
		},
	}
}

func TestNvramItems(t *testing.T) {
	keys := map[string]bool{}
	for _, item := range NvramItems() {
		keys[item.AsHook().Args] = true
	}
	for _, want := range []string{"vpn_clientx_eas", "vpn_client1_state", "vpn_client1_desc", "wgc1_enable", "wgc5_priv"} {
		if !keys[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestEnabledUnits(t *testing.T) {
	if got := enabledUnits(map[string]any{"vpn_clientx_eas": "1,3,"}); !reflect.DeepEqual(got, map[int]bool{1: true, 3: true}) {
		t.Errorf("enabledUnits = %v", got)
	}
	if got := enabledUnits(map[string]any{}); len(got) != 0 {
		t.Errorf("enabledUnits({}) = %v", got)
	}
}

func TestClean(t *testing.T) {
	if clean("None") != nil || clean("") != nil {
		t.Error("placeholders not cleaned")
	}
	if got := clean("10.0.0.1"); got == nil || *got != "10.0.0.1" {
		t.Errorf("clean = %v", got)
	}
}

func TestIP(t *testing.T) {
	// The all-zero address is treated as no address
	if ip("0.0.0.0") != nil || ip("None") != nil {
		t.Error("placeholder parsed")
	}
	if got := ip("10.0.0.1"); got == nil || got.String() != "10.0.0.1" {
		t.Errorf("ip = %v", got)
	}
}

func TestParseStatus(t *testing.T) {
	// Remote endpoint and transfer counters parse to typed fields
	assertCanon(t, parseStatus(fullStatus), map[string]any{
		"remote_address":        "198.51.100.7",
		"remote_port":           1194,
		"connected_since":       "2024-01-02 03:04:05",
		"rx_bytes":              12345,
		"tx_bytes":              678,
		"tun_tap_rx_bytes":      111,
		"tun_tap_tx_bytes":      222,
		"auth_rx_bytes":         333,
		"pre_compress_bytes":    44,
		"post_compress_bytes":   55,
		"pre_decompress_bytes":  66,
		"post_decompress_bytes": 77,
	})

	// A remote with no port maps only the address
	assertCanon(t, parseStatus("REMOTE,198.51.100.7,udp>END"), map[string]any{
		"remote_address": "198.51.100.7",
	})

	// A `None` status yields no fields
	if got := parseStatus("None"); len(got) != 0 {
		t.Errorf("parseStatus(None) = %v", got)
	}
}

func TestTranslate(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		// A device with no configured clients yields an empty result
		idle := map[string]any{StatusKey: map[string]any{"vpn_client1_status": "None"}}
		if got := Translate(idle); len(got) != 0 {
			t.Errorf("Translate(idle) = %v", got)
		}
	})

	t.Run("openvpn profile", func(t *testing.T) {
		// The OpenVPN client carries nvram config, state and live stats
		ovpn := Translate(data())[vpn.ARVpnProtocolOpenvpn][1]
		got := canon(ovpn).(map[string]any)
		for k, want := range map[string]any{
			"name":           "MyOVPN",
			"server":         "vpn.host.test",
			"username":       "bob",
			"state":          float64(2),
			"unit":           float64(1),
			"enabled":        true,
			"address":        "10.8.0.6",
			"remote_address": "198.51.100.7",
			"rx_bytes":       float64(12345),
			"protocol":       "openvpn",
			"endpoint_port":  float64(1194),
		} {
			if !reflect.DeepEqual(got[k], want) {
				t.Errorf("%s = %v, want %v", k, got[k], want)
			}
		}
	})

	t.Run("openvpn error reason", func(t *testing.T) {
		// A failed OpenVPN client records the errno reason
		ovpn := Translate(map[string]any{"vpn_client1_state": "-1", "vpn_client1_errno": "2"})[vpn.ARVpnProtocolOpenvpn][1]
		if ovpn[vpn.ARVpnClientFieldState] != vpn.ARVpnStateError || ovpn[vpn.ARVpnClientFieldStateReason] != 2 {
			t.Errorf("profile = %v", ovpn)
		}
	})

	t.Run("wireguard profile", func(t *testing.T) {
		// The WireGuard client carries its `wgc` config and enable flag
		wg := Translate(data())[vpn.ARVpnProtocolWireguard][2]
		assertCanon(t, wg, map[string]any{
			"protocol":         "wireguard",
			"unit":             2,
			"enabled":          true,
			"address":          "10.6.0.2/32",
			"public_key":       "cHViZmFrZQ==",
			"endpoint_address": "203.0.113.9",
			"endpoint_port":    51820,
		})
	})

	t.Run("disabled wireguard still listed", func(t *testing.T) {
		// A configured-but-disabled WireGuard unit is still reported
		wg := Translate(map[string]any{"wgc3_enable": "0"})[vpn.ARVpnProtocolWireguard][3]
		if wg[vpn.ARVpnClientFieldEnabled] != false {
			t.Errorf("profile = %v", wg)
		}
	})
}

func TestBuildTogglePayload(t *testing.T) {
	services, arguments, ok := BuildTogglePayload(vpn.ARVpnProtocolOpenvpn, 2, true)
	if !ok || len(services) != 1 || services[0] != "start_vpnclient2" || !reflect.DeepEqual(arguments, [][2]any{{"id", 2}}) {
		t.Errorf("openvpn = %v %v %v", services, arguments, ok)
	}

	services, arguments, ok = BuildTogglePayload(vpn.ARVpnProtocolWireguard, 3, false)
	want := [][2]any{{"wgc_enable", 0}, {"wgc_unit", 3}, {"id", 3}}
	if !ok || services[0] != "stop_wgc 3" || !reflect.DeepEqual(arguments, want) {
		t.Errorf("wireguard = %v %v %v", services, arguments, ok)
	}

	if _, _, ok := BuildTogglePayload(vpn.ARVpnProtocolPptp, 1, true); ok {
		t.Error("pptp toggled")
	}
}
