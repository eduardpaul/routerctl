package network

import (
	"context"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func TestSdnSupported(t *testing.T) {
	// The mtlancfg SDN flag drives SDN support
	cases := []struct {
		name  string
		flags map[support.ARSupportType]any
		want  bool
	}{
		{"true", map[support.ARSupportType]any{support.ARSupportTypeSdn: true}, true},
		{"false", map[support.ARSupportType]any{support.ARSupportTypeSdn: false}, false},
		{"missing", map[support.ARSupportType]any{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertEqual(t, sdnFlag(tc.flags), tc.want)
		})
	}

	// No identity means no SDN
	assertEqual(t, sdnSupported(nil), false)
}

func TestFetchStateNoIdentity(t *testing.T) {
	// Without an identity: legacy backend with no bands, so no fetch
	rec := &recorder{}
	got, err := fetchState(context.Background(), &registry.Context{Read: rec.read}, ARNetworkSourceUniversal)
	if err != nil || got != nil || len(rec.calls) != 0 {
		t.Errorf("got %v, %v, calls %d", got, err, len(rec.calls))
	}
}

func TestTranslateStateBadData(t *testing.T) {
	// Non-dict data yields an empty result
	for _, data := range []any{nil, "text", 42} {
		got := translateState(data, &registry.TranslateContext{})
		assertEqual(t, got, map[ARNetworkType][]map[ARNetworkField]any{})
	}
}

func TestTranslateStateBackend(t *testing.T) {
	wifiMap := map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0}

	// Data with sdn_rl uses the SDN parser
	data := map[string]any{"sdn_rl": enc("<1>MAINFH>1>0>0>2"), "apm2_ssid": "Net"}
	if _, ok := translateWifi(data, wifiMap, nil)[ARNetworkTypeMainfh]; !ok {
		t.Error("SDN data: missing MAINFH")
	}

	// Data without sdn_rl uses the legacy parser
	data = map[string]any{"wl0_ssid": "Net", "wl0_radio": "1"}
	net := translateWifi(data, wifiMap, nil)[ARNetworkTypeMainfh][0]
	assertCanon(t, net[ARNetworkFieldSsid], identifiers.SsidFromValueSafe("Net"))
}

func TestSourceIsRegistered(t *testing.T) {
	// The source registers fetch_state and translate_state
	entry := registry.Source(ARNetworkSourceUniversal)
	if entry == nil || entry.Fetch == nil || entry.Translate == nil {
		t.Fatalf("entry %+v", entry)
	}
	assertEqual(t, ARNetworkSourceUniversal.SourceName(), "ARNetworkSource")
}
