package network

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

var sdnBands = []wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1, wifi.ARWiFiBandBand5g1, wifi.ARWiFiBandBand6g1}

// idx 0 (skipped), MAINFH -> apm2, Customized -> apg4
var sdnRl = enc("<0>DEFAULT>1>0>0>0<1>MAINFH>1>0>0>2<3>Customized>1>0>0>4")

// security: band 3 (2.4+5) psk2sae, band 16 (6G) sae
var sdnSecurity = enc("<3>psk2sae>aes>secret>0<16>sae>aes>secret>0")

func sdnAPData() map[string]any {
	return map[string]any{
		"sdn_rl":          sdnRl,
		"apm2_enable":     "1",
		"apm2_ssid":       "MainNet",
		"apm2_hide_ssid":  "0",
		"apm2_security":   sdnSecurity,
		"apm2_ap_isolate": "0",
		"apm2_macmode":    "disabled",
		"apm2_mlo":        "2",
		"apm2_maclist":    enc("<" + testMAC + ">"),
		"apm2_dut_list":   enc("<*>19>"),
		"apm2_11be":       "1",
		"apm2_timesched":  "1",
		"apm2_sched":      "W1E2",
		"apg4_enable":     "1",
		"apg4_ssid":       "Guest",
		"apg4_security":   enc("<3>psk2>aes>guestpass>0"),
		"apg4_dut_list":   enc("<*>1>"),
		"apg4_bw_limit":   enc("<1>2048>5120"),
	}
}

func TestParseSdnRl(t *testing.T) {
	// Profiles parse with prefix and enable; idx 0 is skipped
	assertEqual(t, parseSdnRl(sdnRl), []sdnProfile{
		{"MAINFH", "apm", 2, 1, true},
		{"Customized", "apg", 4, 3, true},
	})

	// Short or empty rule-lists yield no profiles; a non-numeric apg index is skipped
	for _, raw := range []any{nil, "", enc("<1>X>1"), enc("<1>X>1>0>0>z")} {
		if got := parseSdnRl(raw); len(got) != 0 {
			t.Errorf("parseSdnRl(%v) = %v, want empty", raw, got)
		}
	}
}

func TestParseSecurity(t *testing.T) {
	// Password is network-wide; auth is per-band (6G=SAE)
	password, perBand := parseSecurity(sdnSecurity, sdnBands)
	assertCanon(t, password, identifiers.PasswordFromValueSafe("secret"))
	assertEqual(t, perBand[wifi.ARWiFiBandBand2g1][ARNetworkFieldAuth], wifi.ARWiFiAuthModePsk2sae)
	assertEqual(t, perBand[wifi.ARWiFiBandBand6g1][ARNetworkFieldAuth], wifi.ARWiFiAuthModeSae)
	assertEqual(t, perBand[wifi.ARWiFiBandBand2g1][ARNetworkFieldCipher], "aes")
	assertMissing(t, perBand[wifi.ARWiFiBandBand2g1], ARNetworkFieldPassword)

	// A band with no matching entry is omitted; password still read
	password, perBand = parseSecurity(sdnSecurity, []wifi.ARWiFiBand{wifi.ARWiFiBandBand5g2})
	assertEqual(t, len(perBand), 0)
	assertCanon(t, password, identifiers.PasswordFromValueSafe("secret"))

	// An empty passphrase yields no password
	password, _ = parseSecurity(enc("<3>open>aes>>0"), sdnBands)
	if password != nil {
		t.Errorf("got password %v", password)
	}
}

func TestDutBands(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want []wifi.ARWiFiBand
	}{
		// The band bitmask union maps to the device bands
		{"union", enc("<*>19>"), sdnBands},
		// Only the bits present resolve to bands
		{"partial", enc("<*>1>"), []wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1}},
		// No dut list yields no bands
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertEqual(t, dutBands(tc.raw, sdnBands), tc.want)
		})
	}
}

func TestSchedule(t *testing.T) {
	// No scheduling yields no fields
	assertEqual(t, schedule("0", "", ""), map[ARNetworkField]any{})

	// A weekly schedule keeps the raw schedule string
	result := schedule("1", "W1E2", "")
	assertEqual(t, result[ARNetworkFieldScheduleMode], ARNetworkScheduleScheduled)
	assertEqual(t, result[ARNetworkFieldSchedule], "W1E2")
	assertMissing(t, result, ARNetworkFieldExpiry)

	// A one-time schedule keeps the raw expiry string
	result = schedule("2", "", "100,200")
	assertEqual(t, result[ARNetworkFieldScheduleMode], ARNetworkScheduleOneTime)
	assertEqual(t, result[ARNetworkFieldExpiry], "100,200")
	assertMissing(t, result, ARNetworkFieldSchedule)
}

func TestBandwidth(t *testing.T) {
	// An enabled limiter yields both rates in bits/s
	assertEqual(t, bandwidth(enc("<1>2048>5120")), map[ARNetworkField]any{
		ARNetworkFieldBandwidthLimitDownload: 5120 * 1024,
		ARNetworkFieldBandwidthLimitUpload:   2048 * 1024,
	})
	// A disabled or empty limiter yields no fields
	for _, raw := range []any{enc("<0>>"), "", nil} {
		assertEqual(t, bandwidth(raw), map[ARNetworkField]any{})
	}
}

func TestSdnFetchTwoStep(t *testing.T) {
	// sdn_rl is fetched first, then the referenced AP groups
	rec := &recorder{replies: []any{
		map[string]any{"sdn_rl": sdnRl},
		map[string]any{"apm2_ssid": "MainNet", "apg4_ssid": "Guest"},
	}}
	got, err := SdnFetch(context.Background(), rec.read)
	if err != nil {
		t.Fatal(err)
	}
	result := got.(map[string]any)
	assertEqual(t, result["sdn_rl"], sdnRl)
	assertEqual(t, result["apm2_ssid"], "MainNet")
	assertEqual(t, len(rec.calls), 2)
	assertEqual(t, rec.calls[1].endpoint, endpoint.AREndpointFetchData)
	if !strings.Contains(rec.calls[1].request, "nvram_get(apm2_ssid)") {
		t.Errorf("second request %s", rec.calls[1].request)
	}
}

func TestSdnFetchNoSdnRl(t *testing.T) {
	// A missing sdn_rl returns nil without a second fetch
	rec := &recorder{replies: []any{map[string]any{}}}
	got, err := SdnFetch(context.Background(), rec.read)
	if err != nil || got != nil || len(rec.calls) != 1 {
		t.Errorf("got %v, %v, calls %d", got, err, len(rec.calls))
	}
}

func TestSdnFetchNoProfiles(t *testing.T) {
	// An empty sdn_rl returns only the raw list, no second fetch
	rec := &recorder{replies: []any{map[string]any{"sdn_rl": ""}}}
	got, err := SdnFetch(context.Background(), rec.read)
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, got, map[string]any{"sdn_rl": ""})
	assertEqual(t, len(rec.calls), 1)
}

func TestSdnFetchSdnRl(t *testing.T) {
	// The raw sdn_rl is returned from the response dict
	rec := &recorder{replies: []any{map[string]any{"sdn_rl": sdnRl}}}
	got, _ := SdnFetchSdnRl(context.Background(), rec.read)
	assertEqual(t, got, sdnRl)

	// A missing sdn_rl or non-dict response yields nil
	for _, response := range []any{map[string]any{}, "not-a-dict", nil} {
		rec := &recorder{replies: []any{response}}
		if got, _ := SdnFetchSdnRl(context.Background(), rec.read); got != nil {
			t.Errorf("response %v: got %v", response, got)
		}
	}
}

func TestSdnBuildTogglePayload(t *testing.T) {
	cases := []struct {
		name      string
		handle    ARNetworkHandle
		state     bool
		service   string
		enableKey string
		enable    int
		sdnRl     string
	}{
		// Disabling flips the target profile's sdn_enable and apg enable
		{
			"disable flips enable column",
			ARNetworkHandle{Backend: ARNetworkBackendSdn, SdnIdx: intp(1), ApPrefix: strp("apm"), ApIdx: intp(2)},
			false,
			"restart_wireless;restart_sdn 1;",
			"apm2_enable", 0,
			"<0>DEFAULT>1>0>0>0<1>MAINFH>0>0>0>2<3>Customized>1>0>0>4",
		},
		// Enabling sets the target columns to 1
		{
			"enable sets columns",
			ARNetworkHandle{Backend: ARNetworkBackendSdn, SdnIdx: intp(3), ApPrefix: strp("apg"), ApIdx: intp(4)},
			true,
			"restart_wireless;restart_sdn 3;",
			"apg4_enable", 1,
			"<0>DEFAULT>1>0>0>0<1>MAINFH>1>0>0>2<3>Customized>1>0>0>4",
		},
		// An idx with no matching row leaves sdn_rl untouched
		{
			"unknown idx leaves list unchanged",
			ARNetworkHandle{Backend: ARNetworkBackendSdn, SdnIdx: intp(99), ApPrefix: strp("apg"), ApIdx: intp(4)},
			true,
			"restart_wireless;restart_sdn 99;",
			"apg4_enable", 1,
			"<0>DEFAULT>1>0>0>0<1>MAINFH>1>0>0>2<3>Customized>1>0>0>4",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, arguments := SdnBuildTogglePayload(sdnRl, tc.handle, tc.state)
			assertEqual(t, service, tc.service)
			assertEqual(t, arguments, [][2]any{{"sdn_rl", tc.sdnRl}, {tc.enableKey, tc.enable}})
		})
	}
}

func TestSdnTranslateGroupedByType(t *testing.T) {
	// Networks are grouped by their SDN type
	result := SdnTranslate(sdnAPData(), sdnBands)
	assertEqual(t, len(result), 2)
	for _, netType := range []ARNetworkType{ARNetworkTypeMainfh, ARNetworkTypeCustomized} {
		if _, ok := result[netType]; !ok {
			t.Errorf("missing %s", netType)
		}
	}
}

func TestSdnTranslateMainNetwork(t *testing.T) {
	// The main network maps every field
	main := SdnTranslate(sdnAPData(), sdnBands)[ARNetworkTypeMainfh][0]

	assertCanon(t, main[ARNetworkFieldSsid], identifiers.SsidFromValueSafe("MainNet"))
	assertEqual(t, main[ARNetworkFieldEnabled], true)
	assertEqual(t, main[ARNetworkFieldHidden], false)
	assertEqual(t, main[ARNetworkFieldWifi7], true)
	assertEqual(t, main[ARNetworkFieldApIsolate], false)
	assertEqual(t, main[ARNetworkFieldMlo], 2)
	assertEqual(t, main[ARNetworkFieldScheduleMode], ARNetworkScheduleScheduled)
	assertEqual(t, main[ARNetworkFieldSchedule], "W1E2")
	assertCanon(t, main[ARNetworkFieldMacFilterList], []*identifiers.MacAddress{identifiers.MacAddressFromValueSafe(testMAC)})
	assertEqual(t, main[ARNetworkFieldBands], sdnBands)
	assertCanon(t, main[ARNetworkFieldPassword], identifiers.PasswordFromValueSafe("secret"))
	handle := main[ARNetworkFieldHandle].(ARNetworkHandle)
	want := ARNetworkHandle{Backend: ARNetworkBackendSdn, SdnIdx: intp(1), ApPrefix: strp("apm"), ApIdx: intp(2)}
	if !handle.Equal(want) {
		t.Errorf("handle %s, want %s", handle, want)
	}
	assertMissing(t, main, ARNetworkFieldBandwidthLimitDownload)
	security := main[ARNetworkFieldSecurity].(map[wifi.ARWiFiBand]map[ARNetworkField]any)
	assertEqual(t, security[wifi.ARWiFiBandBand6g1][ARNetworkFieldAuth], wifi.ARWiFiAuthModeSae)
}

func TestSdnTranslateSecurityLimitedToBoundBands(t *testing.T) {
	// Security covers only the bands the network is bound to
	guest := SdnTranslate(sdnAPData(), sdnBands)[ARNetworkTypeCustomized][0]

	assertEqual(t, guest[ARNetworkFieldBands], []wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1})
	security := guest[ARNetworkFieldSecurity].(map[wifi.ARWiFiBand]map[ARNetworkField]any)
	assertEqual(t, len(security), 1)
	if _, ok := security[wifi.ARWiFiBandBand2g1]; !ok {
		t.Error("missing 2g1 security")
	}
	assertMissing(t, guest, ARNetworkFieldScheduleMode)
	assertEqual(t, guest[ARNetworkFieldBandwidthLimitDownload], 5120*1024)
	assertEqual(t, guest[ARNetworkFieldBandwidthLimitUpload], 2048*1024)
}

func TestSdnTranslateEnabledRequiresSdnEnable(t *testing.T) {
	// A network off at the SDN level is not enabled
	data := map[string]any{"sdn_rl": enc("<1>MAINFH>0>0>0>2"), "apm2_enable": "1"}
	net := SdnTranslate(data, sdnBands)[ARNetworkTypeMainfh][0]
	assertEqual(t, net[ARNetworkFieldEnabled], false)
}

func TestSdnTranslateNoBandsWithoutIdentity(t *testing.T) {
	// With no device bands, band-derived fields are omitted
	main := SdnTranslate(sdnAPData(), nil)[ARNetworkTypeMainfh][0]
	assertCanon(t, main[ARNetworkFieldSsid], identifiers.SsidFromValueSafe("MainNet"))
	assertMissing(t, main, ARNetworkFieldBands)
	assertMissing(t, main, ARNetworkFieldSecurity)
}
