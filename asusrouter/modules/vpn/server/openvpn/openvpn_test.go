package openvpn

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
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

// identityWith builds an identity whose firmware is read from the given
// nvram values (`firmver`, `buildno`, `extendno`; nil for absent).
//
// Python sets `identity._firmware` directly; the Go identity is built
// from the raw device data instead.
func identityWith(fwMajor, fwMinor, fwBuild any) *device.ARDeviceIdentity {
	data := device.IdentityData{}
	for item, value := range map[nvram.ARNvramType]any{
		nvram.ARNvramTypeFwMajor: fwMajor,
		nvram.ARNvramTypeFwMinor: fwMinor,
		nvram.ARNvramTypeFwBuild: fwBuild,
	} {
		if value != nil {
			data[source.IDOf(item)] = value
		}
	}
	return device.ARDeviceIdentityBuild(data)
}

// Server settings + one account, mirroring the real nvram shape
func data() map[string]any {
	return map[string]any{
		"VPNServer_enable":  "1",
		"vpn_server_unit":   "1",
		"vpn_server1_state": "2",
		"vpn_server1_errno": "0",
		"vpn_server2_state": "",
		"vpn_server_port":   "1194",
		"vpn_server_proto":  "tcp-server",
		"vpn_server_if":     "tun",
		"vpn_server_crypt":  "tls",
		"vpn_server_igncrt": "0",
		"vpn_server_sn":     "10.8.0.0",
		"vpn_server_nm":     "255.255.255.0",
		"vpn_server_dhcp":   "1",
		"vpn_server_r1":     "192.168.1.50",
		"vpn_server_r2":     "192.168.1.55",
		"vpn_server_local":  "10.8.0.1",
		"vpn_server_remote": "10.8.0.2",
		// HTML-encoded `<Surfie>` account
		"vpn_serverx_clientlist": "&#60Surfie&#62",
	}
}

func TestServerEnabled(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  bool
	}{{"1", true}, {"0", false}, {nil, false}} {
		if got := ServerEnabled(map[string]any{"VPNServer_enable": tc.value}); got != tc.want {
			t.Errorf("ServerEnabled(%v) = %v", tc.value, got)
		}
	}
}

func TestReadClientStatus(t *testing.T) {
	entry := func(name, ip, remote string) map[string]any {
		return map[string]any{"name": name, "vpn_ip": ip, "remote": remote}
	}
	for _, tc := range []struct {
		name    string
		content any
		want    []any // nil -> {}
	}{
		{
			"xml", "<?xml version=\"1.0\" ?>\n<vpnserver>\n192.168.55.23:15620 10.8.0.2 Surfie\n\n</vpnserver>",
			[]any{entry("Surfie", "10.8.0.2", "192.168.55.23:15620")},
		},
		// Two clients
		{
			"two", "<vpnserver>\n1.2.3.4:5 10.0.0.2 a\n6.7.8.9:1 10.0.0.3 b\n</vpnserver>",
			[]any{entry("a", "10.0.0.2", "1.2.3.4:5"), entry("b", "10.0.0.3", "6.7.8.9:1")},
		},
		// Empty server (no connected clients)
		{"empty", "<vpnserver>\n\n</vpnserver>", []any{}},
		// Malformed lines (wrong field count) are skipped
		{"malformed", "<vpnserver>\nonly two\n</vpnserver>", []any{}},
		// No wrapping tag: body parsed directly
		{"untagged", "1.2.3.4:5 10.0.0.2 a", []any{entry("a", "10.0.0.2", "1.2.3.4:5")}},
		{"none", nil, nil},
		{"blank", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ReadClientStatus(tc.content)
			if tc.want == nil {
				assertCanon(t, got, map[string]any{})
				return
			}
			assertCanon(t, got, map[string]any{"connected": tc.want})
		})
	}
}

func TestNvramItems(t *testing.T) {
	keys := map[string]bool{}
	for _, item := range NvramItems() {
		keys[item.AsHook().Args] = true
	}
	for _, want := range []string{"VPNServer_enable", "vpn_serverx_clientlist", "vpn_server1_state", "vpn_server2_errno"} {
		if !keys[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestClients(t *testing.T) {
	result := clients("&#60alice&#62secret&#60bob&#62")
	if result[0][vpn.ARVpnPeerFieldName] != "alice" {
		t.Errorf("name = %v", result[0][vpn.ARVpnPeerFieldName])
	}
	if _, ok := result[0][vpn.ARVpnPeerFieldPassword].(identifiers.Password); !ok {
		t.Errorf("password is %T", result[0][vpn.ARVpnPeerFieldPassword])
	}
	if result[1][vpn.ARVpnPeerFieldName] != "bob" {
		t.Errorf("name = %v", result[1][vpn.ARVpnPeerFieldName])
	}
	if _, ok := result[1][vpn.ARVpnPeerFieldPassword]; ok {
		t.Error("bob has a password")
	}

	for _, raw := range []any{"", nil} {
		if got := clients(raw); len(got) != 0 {
			t.Errorf("clients(%v) = %v", raw, got)
		}
	}
}

func TestConnected(t *testing.T) {
	if got := connected(map[string]any{}); len(got) != 0 {
		t.Errorf("connected({}) = %v", got)
	}

	got := connected(map[string]any{
		ClientStatusKey: map[string]any{
			"connected": []any{
				map[string]any{"name": "a", "vpn_ip": "10.8.0.2"},
				map[string]any{"vpn_ip": "10.8.0.9"},
			},
		},
	})
	if len(got) != 1 || got["a"] == nil {
		t.Errorf("connected = %v", got)
	}
}

func TestLiveFields(t *testing.T) {
	assertCanon(t, liveFields(map[string]any{"vpn_ip": "10.8.0.2", "remote": "192.168.55.23:15620"}), map[string]any{
		"state":          2,
		"address":        []any{"10.8.0.2/32"},
		"remote_address": "192.168.55.23",
		"remote_port":    15620,
	})

	// A remote with no port maps only the address
	assertCanon(t, liveFields(map[string]any{"remote": "192.168.55.23"}), map[string]any{
		"state":          2,
		"remote_address": "192.168.55.23",
	})

	// Missing address/remote leaves only STATE
	assertCanon(t, liveFields(map[string]any{}), map[string]any{"state": 2})
}

func TestApplyStatus(t *testing.T) {
	result := applyStatus(
		[]map[vpn.ARVpnPeerField]any{{vpn.ARVpnPeerFieldName: "alice"}},
		map[string]map[string]any{
			"alice": {"name": "alice"},
			"carol": {"name": "carol", "vpn_ip": "10.8.0.5"},
		},
	)

	if result[0][vpn.ARVpnPeerFieldState] != vpn.ARVpnStateConnected {
		t.Errorf("alice state = %v", result[0][vpn.ARVpnPeerFieldState])
	}
	if result[1][vpn.ARVpnPeerFieldName] != "carol" {
		t.Errorf("appended = %v", result[1])
	}
	assertCanon(t, result[1][vpn.ARVpnPeerFieldAddress], []any{"10.8.0.5/32"})
}

func TestTranslate(t *testing.T) {
	t.Run("active unit", func(t *testing.T) {
		server := Translate(data())[1]
		assertCanon(t, server, map[string]any{
			"state":              2,
			"errno":              0,
			"enabled":            true,
			"port":               1194,
			"protocol":           "tcp-server",
			"interface":          "tun",
			"crypt":              "tls",
			"ignore_certificate": false,
			"subnet":             "10.8.0.0",
			"netmask":            "255.255.255.0",
			"dhcp":               true,
			"pool_start":         "192.168.1.50",
			"pool_end":           "192.168.1.55",
			"local_address":      "10.8.0.1",
			"remote_address":     "10.8.0.2",
			"clients":            []any{map[string]any{"name": "Surfie"}},
		})
	})

	t.Run("inactive unit dropped", func(t *testing.T) {
		result := Translate(data())
		if len(result) != 1 || result[1] == nil {
			t.Errorf("units = %v", result)
		}
	})

	t.Run("inactive unit state only", func(t *testing.T) {
		result := Translate(map[string]any{
			"VPNServer_enable":  "1",
			"vpn_server_unit":   "1",
			"vpn_server1_state": "2",
			"vpn_server2_state": "0",
			"vpn_server_port":   "1194",
		})
		assertCanon(t, result[2], map[string]any{"state": 0})
	})

	t.Run("live status merged", func(t *testing.T) {
		d := data()
		d[ClientStatusKey] = map[string]any{
			"connected": []any{map[string]any{"name": "Surfie", "vpn_ip": "10.8.0.2"}},
		}
		client := Translate(d)[1][vpn.ARVpnServerFieldClients].([]map[vpn.ARVpnPeerField]any)[0]
		if client[vpn.ARVpnPeerFieldState] != vpn.ARVpnStateConnected {
			t.Errorf("state = %v", client[vpn.ARVpnPeerFieldState])
		}
	})
}

func TestIsLegacy(t *testing.T) {
	// The nvram-built Merlin firmware is the Python ARFirmware(major=(3, 0,
	// 0, 4), minor=386, build=0, revision="beta")
	if fw := identityWith("3.0.0.4", "386", "0_beta").Firmware(); fw.FirmwareType() != firmware.ARFirmwareTypeMerlin {
		t.Fatalf("firmware type = %v", fw.FirmwareType())
	}

	for _, tc := range []struct {
		name     string
		identity *device.ARDeviceIdentity
		want     bool
	}{
		// No identity falls back to legacy services
		{"none identity", nil, true},
		// Unknown firmware (non-stock) is legacy
		{"unknown firmware", identityWith(nil, nil, nil), true},
		// Merlin-like firmware is legacy
		{"merlin", identityWith("3.0.0.4", "386", "0_beta"), true},
		// Stock firmware older than 388 is legacy
		{"stock below 388", identityWith("3.0.0.4", "384", "0"), true},
		// Stock firmware 388 or newer is not legacy
		{"modern stock", identityWith("3.0.0.4", "388", "0"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLegacy(tc.identity); got != tc.want {
				t.Errorf("isLegacy = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildTogglePayload(t *testing.T) {
	modern := identityWith("3.0.0.4", "388", "0")
	for _, tc := range []struct {
		name      string
		unit      int
		state     bool
		identity  *device.ARDeviceIdentity
		services  []service.ARServiceInput
		arguments [][2]any
	}{
		// Legacy enable uses the per-unit start service
		{"legacy enable", 2, true, nil, []service.ARServiceInput{"start_vpnserver2"}, [][2]any{{"id", 2}}},
		// Legacy disable uses the per-unit stop service
		{"legacy disable", 1, false, nil, []service.ARServiceInput{"stop_vpnserver1"}, [][2]any{{"id", 1}}},
		// Modern enable chains the service restarts and flags nvram on
		{"modern enable", 1, true, modern, []service.ARServiceInput{
			common.ARServiceOpenvpnRestart,
			common.ARServiceChpassRestart,
			common.ARServiceSambaRestart,
			common.ARServiceDnsRestart,
		}, [][2]any{{"VPNServer_enable", 1}, {"id", 1}}},
		// Modern disable stops the daemon and flags nvram off
		{"modern disable", 1, false, modern, []service.ARServiceInput{
			common.ARServiceOpenvpnStop,
			common.ARServiceSambaRestart,
			common.ARServiceDnsRestart,
		}, [][2]any{{"VPNServer_enable", 0}, {"id", 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			services, arguments := BuildTogglePayload(tc.unit, tc.state, tc.identity)
			if !reflect.DeepEqual(services, tc.services) {
				t.Errorf("services = %v, want %v", services, tc.services)
			}
			if !reflect.DeepEqual(arguments, tc.arguments) {
				t.Errorf("arguments = %v, want %v", arguments, tc.arguments)
			}
		})
	}
}
