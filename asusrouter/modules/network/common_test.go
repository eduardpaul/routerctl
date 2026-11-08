package network

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

const testMAC = "AA:BB:CC:DD:EE:FF"

func TestDecode(t *testing.T) {
	// The char entities decode to angle brackets
	assertEqual(t, readers.Decode("&#60a&#62b"), "<a>b")
	// A non-string decodes to empty
	assertEqual(t, readers.Decode(nil), "")
}

func TestReadMacList(t *testing.T) {
	// MACs are extracted from the encoded list
	assertCanon(t, ReadMacList("&#60"+testMAC+"&#62"), []*identifiers.MacAddress{identifiers.MacAddressFromValueSafe(testMAC)})
	// A list with no MACs is empty
	if got := ReadMacList(""); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestMacFilterMode(t *testing.T) {
	known := []struct {
		raw  string
		want wifi.ARWiFiMacFilterMode
	}{
		{"0", wifi.ARWiFiMacFilterModeDisabled},
		{"disabled", wifi.ARWiFiMacFilterModeDisabled},
		{"1", wifi.ARWiFiMacFilterModeAllow},
		{"allow", wifi.ARWiFiMacFilterModeAllow},
		{"2", wifi.ARWiFiMacFilterModeDeny},
		{"deny", wifi.ARWiFiMacFilterModeDeny},
	}
	for _, tc := range known {
		got := MacFilterMode(tc.raw)
		if got == nil || *got != tc.want {
			t.Errorf("MacFilterMode(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
	for _, raw := range []any{"x", nil} {
		if got := MacFilterMode(raw); got != nil {
			t.Errorf("MacFilterMode(%v) = %v, want nil", raw, *got)
		}
	}
}

func TestBandwidthLimit(t *testing.T) {
	// Enabled limiter yields both rates in bits/s (from Kib/s)
	assertEqual(t, BandwidthLimit("1", "5120", "2048"), map[ARNetworkField]any{
		ARNetworkFieldBandwidthLimitDownload: 5120 * 1024,
		ARNetworkFieldBandwidthLimitUpload:   2048 * 1024,
	})
	// A disabled or missing limiter yields no fields
	for _, enabled := range []any{"0", nil, ""} {
		assertEqual(t, BandwidthLimit(enabled, "5120", "2048"), map[ARNetworkField]any{})
	}
	// Enabled with unparsable rates yields no rate fields
	assertEqual(t, BandwidthLimit("1", nil, nil), map[ARNetworkField]any{})
}
