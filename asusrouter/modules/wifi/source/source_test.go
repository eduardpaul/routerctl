package source

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const testMac = "AA:BB:CC:00:11:22"

type bandUnit struct {
	band wifi.ARWiFiBand
	unit int
}

// identity builds an identity carrying the given wifi map (Python uses a
// SimpleNamespace stub): WIRELESS_BANDS plus the WiFi units support.
func identity(bands ...bandUnit) *device.ARDeviceIdentity {
	if len(bands) == 0 {
		return device.NewARDeviceIdentity()
	}
	rows := make([]string, len(bands))
	units := make([]int, len(bands))
	for i, b := range bands {
		rows[i] = string(b.band)
		units[i] = b.unit
	}
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(nvram.ARNvramTypeWirelessBands): strings.Join(rows, "&#60"),
		source.IDOf(support.ARSupportSourceUniversal): map[support.ARSupportType]any{
			support.ARSupportTypeWifiCapabilities: map[wifi.ARWiFiCapability]any{
				wifi.ARWiFiCapabilityUnits: units,
			},
		},
	})
}

// Router with three radios: 2g1 (unit 0), 5g1 (unit 1), 6g1 (unit 2)
var threeRadios = []bandUnit{
	{wifi.ARWiFiBandBand2g1, 0},
	{wifi.ARWiFiBandBand5g1, 1},
	{wifi.ARWiFiBandBand6g1, 2},
}

var response = map[string]any{
	"wl_control_channel": []any{"6", "100", "5"},
	"2g1_nmode_x":        "0",
	"2g1_bw":             "5",
	"2g1_chanspec":       "6u",
	"2g1_nctrlsb":        "",
	"2g1_bw_160":         "0",
	"2g1_bw_240":         "",
	"2g1_11be":           "1",
	"wl0_radio":          "1",
	"wl0_country_code":   "US",
	"wl0_version":        "17.10",
	"wl0_hwaddr":         testMac,
}

func TestIdentityHelper(t *testing.T) {
	got := identity(threeRadios...).Wifi()
	want := map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0, wifi.ARWiFiBandBand5g1: 1, wifi.ARWiFiBandBand6g1: 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identity wifi = %v, want %v", got, want)
	}
}

func TestArrayAt(t *testing.T) {
	tests := []struct {
		name  string
		value any
		index int
		want  any
	}{
		{"InRange", []any{"a", "b", "c"}, 1, "b"},
		{"OutOfRange", []any{"a"}, 5, nil},
		{"Negative", []any{"a"}, -1, nil},
		{"NotAList", "not a list", 0, nil},
		{"None", nil, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arrayAt(tt.value, tt.index); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildRequest(t *testing.T) {
	t.Run("NoIdentity", func(t *testing.T) {
		if got := buildRequest(nil); got != "" {
			t.Errorf("got %q, want no request", got)
		}
	})
	t.Run("EmptyWifi", func(t *testing.T) {
		if got := buildRequest(identity()); got != "" {
			t.Errorf("got %q, want no request", got)
		}
	})
	t.Run("BuildsRequest", func(t *testing.T) {
		request := buildRequest(identity(bandUnit{wifi.ARWiFiBandBand2g1, 0}))
		if !strings.HasPrefix(request, "hook=wl_control_channel();") {
			t.Errorf("request = %q", request)
		}
		for _, part := range []string{"nvram_get(2g1_bw)", "nvram_get(2g1_chanspec)", "nvram_get(wl0_hwaddr)"} {
			if !strings.Contains(request, part) {
				t.Errorf("request misses %s: %q", part, request)
			}
		}
	})
}

type call struct {
	ep      endpoint.AREndpoint
	request string
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()
	newReader := func(calls *[]call) registry.ReadFunc {
		return func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
			*calls = append(*calls, call{ep, request})
			return map[string]any{"ok": true}, nil
		}
	}
	t.Run("NoIdentity", func(t *testing.T) {
		var calls []call
		result, err := fetchState(ctx, &registry.Context{Read: newReader(&calls)}, ARWiFiSourceUniversal)
		if err != nil {
			t.Fatal(err)
		}
		if m, ok := result.(map[string]any); !ok || len(m) != 0 {
			t.Errorf("result = %v, want {}", result)
		}
		if len(calls) != 0 {
			t.Errorf("calls = %v, want none", calls)
		}
	})
	t.Run("Fetches", func(t *testing.T) {
		var calls []call
		c := &registry.Context{Read: newReader(&calls), Identity: identity(bandUnit{wifi.ARWiFiBandBand2g1, 0})}
		result, err := fetchState(ctx, c, ARWiFiSourceUniversal)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(result, map[string]any{"ok": true}) {
			t.Errorf("result = %v", result)
		}
		if len(calls) != 1 || calls[0].ep != endpoint.AREndpointFetchData || !strings.HasPrefix(calls[0].request, "hook=") {
			t.Errorf("calls = %v", calls)
		}
	})
}

func translate(data any, id *device.ARDeviceIdentity) map[wifi.ARWiFiBand]map[wifi.ARWiFiField]any {
	tc := &registry.TranslateContext{}
	if id != nil {
		tc.Identity = id
	}
	return translateState(data, tc).(map[wifi.ARWiFiBand]map[wifi.ARWiFiField]any)
}

func TestTranslateState(t *testing.T) {
	for _, tt := range []struct {
		name string
		data any
	}{
		{"BadData/None", nil},
		{"BadData/Text", "text"},
		{"BadData/Int", 42},
		{"BadData/List", []any{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := translate(tt.data, identity(threeRadios...)); len(got) != 0 {
				t.Errorf("got %v, want {}", got)
			}
		})
	}
	t.Run("NoIdentity", func(t *testing.T) {
		if got := translate(response, nil); len(got) != 0 {
			t.Errorf("got %v, want {}", got)
		}
		if got := translateState(response, nil).(map[wifi.ARWiFiBand]map[wifi.ARWiFiField]any); len(got) != 0 {
			t.Errorf("nil context: got %v, want {}", got)
		}
	})
	t.Run("EmptyWifi", func(t *testing.T) {
		if got := translate(response, identity()); len(got) != 0 {
			t.Errorf("got %v, want {}", got)
		}
	})
	t.Run("FullBand", func(t *testing.T) {
		band := translate(response, identity(threeRadios...))[wifi.ARWiFiBandBand2g1]
		mac, _ := identifiers.MacAddressFromValue(testMac)
		want := map[wifi.ARWiFiField]any{
			wifi.ARWiFiFieldChannel:      6,
			wifi.ARWiFiFieldWirelessMode: 0,
			wifi.ARWiFiFieldBandwidth:    wifi.ARWiFiBandwidthWidth160,
			wifi.ARWiFiFieldChannelSpec:  "6u",
			wifi.ARWiFiFieldEnable160mhz: false,
			wifi.ARWiFiFieldWifi7:        true,
			wifi.ARWiFiFieldState:        true,
			wifi.ARWiFiFieldCountry:      "US",
			wifi.ARWiFiFieldDriver:       "17.10",
			wifi.ARWiFiFieldMac:          mac,
		}
		for field, w := range want {
			if got := band[field]; !reflect.DeepEqual(got, w) {
				t.Errorf("%s = %#v, want %#v", field, got, w)
			}
		}
	})
	t.Run("EmptyFieldsSkipped", func(t *testing.T) {
		band := translate(response, identity(threeRadios...))[wifi.ARWiFiBandBand2g1]
		for _, field := range []wifi.ARWiFiField{wifi.ARWiFiFieldSideBand, wifi.ARWiFiFieldEnable240mhz} {
			if _, ok := band[field]; ok {
				t.Errorf("%s present, want absent", field)
			}
		}
	})
	t.Run("HookOnlyBand", func(t *testing.T) {
		band := translate(response, identity(threeRadios...))[wifi.ARWiFiBandBand5g1]
		if want := (map[wifi.ARWiFiField]any{wifi.ARWiFiFieldChannel: 100}); !reflect.DeepEqual(band, want) {
			t.Errorf("band = %v, want %v", band, want)
		}
	})
	t.Run("BandlessUnitOmitted", func(t *testing.T) {
		if got := translate(response, identity(bandUnit{wifi.ARWiFiBandBand6g2, 9})); len(got) != 0 {
			t.Errorf("got %v, want {}", got)
		}
	})
}

func TestSourceIsRegistered(t *testing.T) {
	e := registry.SourceByName("ARWiFiSource")
	if e == nil || e.Fetch == nil || e.Translate == nil {
		t.Fatal("ARWiFiSource must register fetch_state and translate_state")
	}
	if registry.Source(ARWiFiSourceUniversal) != e {
		t.Error("the universal instance resolves to another entry")
	}
}
