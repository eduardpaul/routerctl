package fusion

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

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

func intp(v int) *int { return &v }

// Three profiles (fake identifiers): a connected WireGuard client, a
// connecting OpenVPN client on the default WAN, and a failed, inactive
// PPTP client
func data() map[string]any {
	return map[string]any{
		// The device HTML-encodes the `<` / `>` list delimiters (&#60 / &#62)
		"get_vpnc_status":               "2&#620&#625&#601&#620&#626&#604&#622&#627",
		"get_vpnc_nondef_wan_prof_list": "5&#620&#606&#621&#607&#620",
		"vpnc_default_wan":              "6",
		"vpnc_pptp_options_x_list":      "&#60auto&#60auto&#60dhcp",
		"vpnc_clientlist": "Home/WG&#62WireGuard&#622&#62&#62&#621&#625&#62&#62&#620&#620&#62Web" +
			"&#60OVPN&#62OpenVPN&#623&#62user1&#62pass1&#621&#626" +
			"&#62region1&#62&#620&#620&#62Web" +
			"&#60MyPPTP&#62PPTP&#62vpn.example.test&#62puser&#62ppass" +
			"&#620&#627&#62&#62&#620&#620&#62Web",
		"wgc2_priv":    "cHJpdmZha2U=",
		"wgc2_addr":    "10.9.0.2/32",
		"wgc2_dns":     "10.9.0.1",
		"wgc2_mtu":     "1420",
		"wgc2_ppub":    "cHViZmFrZQ==",
		"wgc2_psk":     "",
		"wgc2_aips":    "0.0.0.0/0",
		"wgc2_ep_addr": "203.0.113.5",
		"wgc2_ep_port": "51820",
		"wgc2_alive":   "25",
		"wgc2_nat":     "1",
	}
}

func TestNvramItems(t *testing.T) {
	keys := map[string]bool{}
	for _, item := range NvramItems() {
		keys[item.AsHook().Args] = true
	}
	for _, want := range []string{"vpnc_clientlist", "vpnc_default_wan", "wgc1_priv", fmt.Sprintf("wgc%d_nat", WGUnits[len(WGUnits)-1])} {
		if !keys[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestStatus(t *testing.T) {
	// Each status row maps vpnc_idx to (state, reason)
	got := readStatus(data())
	want := map[int]statusEntry{
		5: {vpn.ARVpnStateConnected, intp(0)},
		6: {vpn.ARVpnStateConnecting, intp(0)},
		7: {vpn.ARVpnStateError, intp(2)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("status = %v", got)
	}

	// Rows without a vpnc_idx are skipped; unknown codes are UNKNOWN
	got = readStatus(map[string]any{"get_vpnc_status": "2>0><9>0>3"})
	if len(got) != 1 || got[3].state != vpn.ARVpnStateUnknown {
		t.Errorf("status = %v", got)
	}

	// No status hook yields an empty map
	if got := readStatus(map[string]any{}); len(got) != 0 {
		t.Errorf("status = %v", got)
	}
}

func TestDefaultWanSupport(t *testing.T) {
	// `0` means allowed on the default WAN; anything else is not
	if got := defaultWanSupport(data()); !reflect.DeepEqual(got, map[int]bool{5: true, 6: false, 7: true}) {
		t.Errorf("support = %v", got)
	}
	// No hook yields an empty map
	if got := defaultWanSupport(map[string]any{}); len(got) != 0 {
		t.Errorf("support = %v", got)
	}
	// Rows without a numeric vpnc_idx are skipped
	if got := defaultWanSupport(map[string]any{"get_vpnc_nondef_wan_prof_list": ">0<5>0"}); !reflect.DeepEqual(got, map[int]bool{5: true}) {
		t.Errorf("support = %v", got)
	}
}

func TestWireguard(t *testing.T) {
	// The `wgc{unit}_*` config maps to typed fields; empty psk absent
	fields := wireguard(data(), 2)
	if _, ok := fields[vpn.ARVpnClientFieldPrivateKey].(identifiers.Password); !ok {
		t.Errorf("private key is %T", fields[vpn.ARVpnClientFieldPrivateKey])
	}
	want := map[string]any{
		"private_key":      "cHJpdmZha2U=",
		"address":          "10.9.0.2/32",
		"dns":              "10.9.0.1",
		"mtu":              float64(1420),
		"public_key":       "cHViZmFrZQ==",
		"allowed_ips":      []any{"0.0.0.0/0"},
		"endpoint_address": "203.0.113.5",
		"endpoint_port":    float64(51820),
		"keepalive":        float64(25),
		"nat":              true,
	}
	if got := canon(fields); !reflect.DeepEqual(got, want) {
		t.Errorf("fields = %v", got)
	}

	// An absent unit yields no fields
	if got := wireguard(data(), 4); len(got) != 0 {
		t.Errorf("fields = %v", got)
	}
}

func TestTranslate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := Translate(map[string]any{}); len(got) != 0 {
			t.Errorf("Translate({}) = %v", got)
		}
	})

	t.Run("groups by protocol", func(t *testing.T) {
		// Profiles group by protocol, keyed by vpnc_idx
		result := Translate(data())
		if len(result) != 3 || result[vpn.ARVpnProtocolPptp] == nil || result[vpn.ARVpnProtocolOpenvpn] == nil {
			t.Errorf("protocols = %v", result)
		}
		if wg := result[vpn.ARVpnProtocolWireguard]; len(wg) != 1 || wg[5] == nil {
			t.Errorf("wireguard = %v", wg)
		}
	})

	t.Run("wireguard profile", func(t *testing.T) {
		// The WireGuard profile carries three ids, status and peer config
		wg := canon(Translate(data())[vpn.ARVpnProtocolWireguard][5]).(map[string]any)
		for k, want := range map[string]any{
			"name":                "Home/WG",
			"unit":                float64(0), // position / control unit
			"server":              float64(2), // per-protocol unit
			"vpnc_index":          float64(5), // status key
			"enabled":             true,
			"state":               float64(2),
			"default_wan_support": true,
			"default_wan":         false,
			"allowed_ips":         []any{"0.0.0.0/0"},
		} {
			if !reflect.DeepEqual(wg[k], want) {
				t.Errorf("%s = %v, want %v", k, wg[k], want)
			}
		}
		// Connected, no reason
		if _, ok := wg["state_reason"]; ok {
			t.Error("state_reason present")
		}
	})

	t.Run("openvpn profile", func(t *testing.T) {
		// The OpenVPN profile carries clientlist auth and default WAN
		profile := Translate(data())[vpn.ARVpnProtocolOpenvpn][6]
		if _, ok := profile[vpn.ARVpnClientFieldPassword].(identifiers.Password); !ok {
			t.Errorf("password is %T", profile[vpn.ARVpnClientFieldPassword])
		}
		ovpn := canon(profile).(map[string]any)
		for k, want := range map[string]any{
			"server":              float64(3),
			"username":            "user1",
			"region":              "region1",
			"state":               float64(1),
			"default_wan":         true,
			"default_wan_support": false,
		} {
			if !reflect.DeepEqual(ovpn[k], want) {
				t.Errorf("%s = %v, want %v", k, ovpn[k], want)
			}
		}
		// OpenVPN keeps no wgc peer config
		if _, ok := ovpn["private_key"]; ok {
			t.Error("private_key present")
		}
	})

	t.Run("pptp profile", func(t *testing.T) {
		// PPTP keeps a host server, custom options, error state and reason
		pptp := canon(Translate(data())[vpn.ARVpnProtocolPptp][7]).(map[string]any)
		for k, want := range map[string]any{
			"server":       "vpn.example.test",
			"enabled":      false,
			"pptp_options": "dhcp",
			"state":        float64(-1),
			"state_reason": float64(2),
		} {
			if !reflect.DeepEqual(pptp[k], want) {
				t.Errorf("%s = %v, want %v", k, pptp[k], want)
			}
		}
	})

	t.Run("skips empty rows", func(t *testing.T) {
		// Empty clientlist rows are skipped without advancing protocols
		d := data()
		d["vpnc_clientlist"] = "<OVPN>OpenVPN>3>u>p>1>6>>>0>0>Web<"
		if result := Translate(d); len(result) != 1 || result[vpn.ARVpnProtocolOpenvpn] == nil {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("skips nameless row", func(t *testing.T) {
		// A row with an empty name is dropped
		d := data()
		d["vpnc_clientlist"] = ">OpenVPN>3>u>p>1>6>>>0>0>Web"
		if result := Translate(d); len(result) != 0 {
			t.Errorf("result = %v", result)
		}
	})
}

func TestBuildTogglePayload(t *testing.T) {
	// A leading empty clientlist row is skipped when locating the unit
	_, arguments, ok := BuildTogglePayload("<Prtn>OpenVPN>4>u>p>0>6>>>0>0>Web", vpn.ARVpnProtocolOpenvpn, 4, true)
	if !ok || arguments[0] != [2]any{"vpnc_unit", 0} {
		t.Fatalf("arguments = %v, ok = %v", arguments, ok)
	}
	if !strings.Contains(arguments[1][1].(string), ">1>6>") {
		t.Errorf("clientlist = %v", arguments[1][1])
	}

	// A protocol/unit not in the clientlist yields nothing
	if _, _, ok := BuildTogglePayload("A>OpenVPN>1>>>0>2>", vpn.ARVpnProtocolWireguard, 1, true); ok {
		t.Error("missing unit toggled")
	}
}
