package network

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

var legacyWifi = map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0, wifi.ARWiFiBandBand5g1: 1}

// Main "HomeNet" shared across both bands (smart connect); one 2.4G guest
func legacyData() map[string]any {
	return map[string]any{
		"wl0_ssid":          "HomeNet",
		"wl0_wpa_psk":       "pass1234",
		"wl0_auth_mode_x":   "psk2",
		"wl0_crypto":        "aes",
		"wl0_closed":        "1",
		"wl0_radio":         "1",
		"wl0_macmode":       "disabled",
		"wl0_maclist_x":     "&#60" + testMAC + "&#62",
		"wl0_ap_isolate":    "0",
		"wl1_ssid":          "HomeNet",
		"wl1_wpa_psk":       "pass1234",
		"wl1_auth_mode_x":   "psk2",
		"wl1_crypto":        "aes",
		"wl1_radio":         "1",
		"wl1.1_ssid":        "",
		"wl0.1_ssid":        "Guest",
		"wl0.1_wpa_psk":     "guestpw",
		"wl0.1_auth_mode_x": "open",
		"wl0.1_bss_enabled": "1",
		"wl0.1_lanaccess":   "off",
		"wl0.1_expire":      "3600",
		"wl0.1_expire_tmp":  "1200",
		"wl0.1_bw_enabled":  "1",
		"wl0.1_bw_dl":       "5120",
		"wl0.1_bw_ul":       "2048",
	}
}

// recorder is a ReadFunc recording its calls.
type recorder struct {
	calls   []recordedCall
	replies []any
}

type recordedCall struct {
	endpoint endpoint.AREndpoint
	request  string
}

func (r *recorder) read(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
	r.calls = append(r.calls, recordedCall{ep, request})
	if len(r.replies) == 0 {
		return map[string]any{}, nil
	}
	reply := r.replies[0]
	if len(r.replies) > 1 {
		r.replies = r.replies[1:]
	}
	return reply, nil
}

func TestLegacyFetchNoWifi(t *testing.T) {
	// No wifi map returns nil without a fetch
	rec := &recorder{}
	got, err := LegacyFetch(context.Background(), rec.read, map[wifi.ARWiFiBand]int{}, nil)
	if err != nil || got != nil || len(rec.calls) != 0 {
		t.Errorf("got %v, %v, calls %d", got, err, len(rec.calls))
	}
}

func TestLegacyFetchRequest(t *testing.T) {
	// The request covers main and guest keys for every band
	rec := &recorder{replies: []any{map[string]any{"wl0_ssid": "X"}}}
	if _, err := LegacyFetch(context.Background(), rec.read, legacyWifi, nil); err != nil {
		t.Fatal(err)
	}
	call := rec.calls[len(rec.calls)-1]
	assertEqual(t, call.endpoint, endpoint.AREndpointFetchData)
	for _, want := range []string{"nvram_get(wl0_ssid)", "nvram_get(wl1_ssid)", "nvram_get(wl0.1_ssid)"} {
		if !strings.Contains(call.request, want) {
			t.Errorf("request misses %s", want)
		}
	}
}

func TestLegacyTranslateMainGroupedBySsid(t *testing.T) {
	// The shared main SSID groups both bands into one network
	main := LegacyTranslate(legacyData(), legacyWifi, nil)[ARNetworkTypeMainfh]

	if len(main) != 1 {
		t.Fatalf("got %d networks", len(main))
	}
	net := main[0]
	assertCanon(t, net[ARNetworkFieldSsid], identifiers.SsidFromValueSafe("HomeNet"))
	assertEqual(t, net[ARNetworkFieldEnabled], true)
	assertEqual(t, net[ARNetworkFieldHidden], true)
	assertCanon(t, net[ARNetworkFieldPassword], identifiers.PasswordFromValueSafe("pass1234"))
	assertEqual(t, net[ARNetworkFieldBands], []wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1, wifi.ARWiFiBandBand5g1})
	assertCanon(t, net[ARNetworkFieldMacFilterList], []*identifiers.MacAddress{identifiers.MacAddressFromValueSafe(testMAC)})
	security := net[ARNetworkFieldSecurity].(map[wifi.ARWiFiBand]map[ARNetworkField]any)
	assertEqual(t, security[wifi.ARWiFiBandBand2g1][ARNetworkFieldAuth], wifi.ARWiFiAuthModePsk2)
	assertEqual(t, security[wifi.ARWiFiBandBand5g1][ARNetworkFieldCipher], "aes")
}

func TestLegacyTranslateGuest(t *testing.T) {
	// A guest slot becomes a GUEST network on its band
	guests := LegacyTranslate(legacyData(), legacyWifi, nil)[ARNetworkTypeGuest]

	if len(guests) != 1 {
		t.Fatalf("got %d guests", len(guests))
	}
	guest := guests[0]
	assertCanon(t, guest[ARNetworkFieldSsid], identifiers.SsidFromValueSafe("Guest"))
	assertEqual(t, guest[ARNetworkFieldEnabled], true)
	assertEqual(t, guest[ARNetworkFieldBands], []wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1})
	assertEqual(t, guest[ARNetworkFieldLanAccess], false)
	assertEqual(t, guest[ARNetworkFieldExpire], 3600)
	assertEqual(t, guest[ARNetworkFieldExpireRemaining], 1200)
	assertEqual(t, guest[ARNetworkFieldBandwidthLimitDownload], 5120*1024)
	assertEqual(t, guest[ARNetworkFieldBandwidthLimitUpload], 2048*1024)
	security := guest[ARNetworkFieldSecurity].(map[wifi.ARWiFiBand]map[ARNetworkField]any)
	assertEqual(t, security[wifi.ARWiFiBandBand2g1][ARNetworkFieldAuth], wifi.ARWiFiAuthModeOpen)
}

func TestLegacyTranslateMainHasNoGuestFields(t *testing.T) {
	// Guest-only nvram is absent on the main network
	main := LegacyTranslate(legacyData(), legacyWifi, nil)[ARNetworkTypeMainfh][0]

	for _, field := range []ARNetworkField{
		ARNetworkFieldLanAccess,
		ARNetworkFieldExpire,
		ARNetworkFieldExpireRemaining,
		ARNetworkFieldBandwidthLimitDownload,
	} {
		assertMissing(t, main, field)
	}
}

func TestLegacyTranslateMainEnabled(t *testing.T) {
	cases := []struct {
		name   string
		radios map[string]any
		want   bool
	}{
		// A shared main stays enabled while any band radio is on
		{"any band on", map[string]any{"wl0_radio": "0"}, true},
		// A shared main is disabled only when every band radio is off
		{"all bands off", map[string]any{"wl0_radio": "0", "wl1_radio": "0"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := legacyData()
			for k, v := range tc.radios {
				data[k] = v
			}
			net := LegacyTranslate(data, legacyWifi, nil)[ARNetworkTypeMainfh][0]
			assertEqual(t, net[ARNetworkFieldEnabled], tc.want)
		})
	}
}

func TestLegacyTranslateSplitSsid(t *testing.T) {
	// Different per-band SSIDs become separate networks
	data := map[string]any{
		"wl0_ssid":  "Net2G",
		"wl0_radio": "1",
		"wl1_ssid":  "Net5G",
		"wl1_radio": "1",
	}
	main := LegacyTranslate(data, legacyWifi, nil)[ARNetworkTypeMainfh]

	var ssids []any
	for _, n := range main {
		ssids = append(ssids, n[ARNetworkFieldSsid])
	}
	assertCanon(t, ssids, []any{identifiers.SsidFromValueSafe("Net2G"), identifiers.SsidFromValueSafe("Net5G")})
}

func TestLegacyTranslateNoNetworks(t *testing.T) {
	// No configured SSIDs yield an empty result
	assertEqual(t, LegacyTranslate(map[string]any{}, legacyWifi, nil), map[ARNetworkType][]map[ARNetworkField]any{})
}

func TestLegacyTranslateHandles(t *testing.T) {
	result := LegacyTranslate(legacyData(), legacyWifi, nil)

	// The main network carries a legacy handle with per-band units
	main := result[ARNetworkTypeMainfh][0][ARNetworkFieldHandle].(ARNetworkHandle)
	want := ARNetworkHandle{Backend: ARNetworkBackendLegacy, Units: []ARNetworkHandleUnit{{Unit: 0}, {Unit: 1}}}
	if !main.Equal(want) {
		t.Errorf("main handle %s, want %s", main, want)
	}

	// The guest network carries a legacy handle with its unit and slot
	guest := result[ARNetworkTypeGuest][0][ARNetworkFieldHandle].(ARNetworkHandle)
	want = ARNetworkHandle{Backend: ARNetworkBackendLegacy, Units: []ARNetworkHandleUnit{{Unit: 0, Slot: intp(1)}}}
	if !guest.Equal(want) {
		t.Errorf("guest handle %s, want %s", guest, want)
	}
}

func TestLegacyBuildTogglePayload(t *testing.T) {
	cases := []struct {
		name      string
		units     []ARNetworkHandleUnit
		state     bool
		service   string
		arguments [][2]any
	}{
		// A main handle toggles the per-band radio via restart_wireless
		{
			"main toggles radio",
			[]ARNetworkHandleUnit{{Unit: 0}, {Unit: 1}},
			false,
			"restart_wireless",
			[][2]any{{"wl0_radio", 0}, {"wl1_radio", 0}},
		},
		// Enabling a guest sets bss_enabled and clears the time limit
		{
			"guest enable clears expire",
			[]ARNetworkHandleUnit{{Unit: 0, Slot: intp(1)}},
			true,
			"restart_wireless;restart_firewall",
			[][2]any{{"wl0.1_bss_enabled", 1}, {"wl0.1_expire", 0}},
		},
		// Disabling a guest clears bss_enabled without touching expire
		{
			"guest disable keeps expire",
			[]ARNetworkHandleUnit{{Unit: 0, Slot: intp(1)}},
			false,
			"restart_wireless;restart_firewall",
			[][2]any{{"wl0.1_bss_enabled", 0}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handle := ARNetworkHandle{Backend: ARNetworkBackendLegacy, Units: tc.units}
			service, arguments := LegacyBuildTogglePayload(handle, tc.state)
			assertEqual(t, service, tc.service)
			assertEqual(t, arguments, tc.arguments)
		})
	}
}
