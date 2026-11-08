package nvram

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func id(src source.DataSource) source.ID { return source.IDOf(src) }

func TestTranslateState(t *testing.T) {
	t.Run("PreservesUntranslatedValues", func(t *testing.T) {
		data := map[source.ID]any{
			id(ARNvramTypeModel):  "RT-AX88U",
			id(ARNvramTypeSerial): "12345",
		}
		if got := TranslateState(data); !reflect.DeepEqual(got, data) {
			t.Errorf("got %v, want %v", got, data)
		}
	})
	for _, key := range []ARNvramType{ARNvramTypeMac, ARNvramTypeMacLan, ARNvramTypeMacWan} {
		t.Run("ConvertsMacKeysToMacAddress/"+key.Name(), func(t *testing.T) {
			result := TranslateState(map[source.ID]any{id(key): "00:11:22:33:44:55"})
			mac, ok := result[id(key)].(identifiers.MacAddress)
			if !ok {
				t.Fatalf("got %T, want MacAddress", result[id(key)])
			}
			if mac.String() != "00:11:22:33:44:55" {
				t.Errorf("got %s", mac)
			}
		})
	}
	t.Run("MixedTranslatedAndPlain", func(t *testing.T) {
		result := TranslateState(map[source.ID]any{
			id(ARNvramTypeMac):   "AA:BB:CC:11:22:33",
			id(ARNvramTypeModel): "RT-AX88U",
		})
		if _, ok := result[id(ARNvramTypeMac)].(identifiers.MacAddress); !ok {
			t.Errorf("MAC = %T, want MacAddress", result[id(ARNvramTypeMac)])
		}
		if result[id(ARNvramTypeModel)] != "RT-AX88U" {
			t.Errorf("model = %v", result[id(ARNvramTypeModel)])
		}
	})
	t.Run("EmptyInputReturnsEmpty", func(t *testing.T) {
		if got := TranslateState(map[source.ID]any{}); len(got) != 0 {
			t.Errorf("got %v, want {}", got)
		}
	})
}

func TestARNvramIndexSource(t *testing.T) {
	t.Run("Key", func(t *testing.T) {
		if got := NewARNvramIndexSource(ARNvramIndexTypeWanIpaddr, 0).Key(); got != "wan0_ipaddr" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("AsHook", func(t *testing.T) {
		want := endpoint.ARHookCall{Hook: endpoint.ARHookNvramGet, Args: "wan1_ipaddr"}
		if got := NewARNvramIndexSource(ARNvramIndexTypeWanIpaddr, 1).AsHook(); got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("EqualityAndHash", func(t *testing.T) {
		// Go sources are equal by `source.IDOf` (Python __eq__/__hash__)
		a := NewARNvramIndexSource(ARNvramIndexTypeWanState, 1)
		b := NewARNvramIndexSource(ARNvramIndexTypeWanState, 1)
		c := NewARNvramIndexSource(ARNvramIndexTypeWanState, 0)
		if id(a) != id(b) {
			t.Error("a != b")
		}
		if id(a) == id(c) {
			t.Error("a == c")
		}
	})
	t.Run("EqualityOtherType", func(t *testing.T) {
		src := NewARNvramIndexSource(ARNvramIndexTypeWanState, 0)
		if id(src) == source.ID("x") {
			t.Error("a source must not equal a string")
		}
		// A flat member with the same raw text is a different source
		if id(src) == id(ARNvramType("wan0_state_t")) {
			t.Error("indexed and flat sources must differ")
		}
	})
	t.Run("Repr", func(t *testing.T) {
		if got := NewARNvramIndexSource(ARNvramIndexTypeWanGateway, 1).String(); got != "<ARNvramIndexSource wan1_gateway>" {
			t.Errorf("got %q", got)
		}
	})
}

func TestTranslateStateIndexed(t *testing.T) {
	t.Run("IndexedKeyedOnTemplate", func(t *testing.T) {
		state := NewARNvramIndexSource(ARNvramIndexTypeWanState, 0)
		ipaddr := NewARNvramIndexSource(ARNvramIndexTypeWanIpaddr, 0)
		result := TranslateState(map[source.ID]any{id(state): "2", id(ipaddr): "1.2.3.4"})

		if result[id(state)] != common.ARConnectionStatusConnected {
			t.Errorf("state = %#v, want CONNECTED", result[id(state)])
		}
		want, _ := identifiers.IpAddressFromValue("1.2.3.4")
		if !reflect.DeepEqual(result[id(ipaddr)], want) {
			t.Errorf("ipaddr = %#v, want %v", result[id(ipaddr)], want)
		}
	})
	t.Run("IndexedUntranslatedPreserved", func(t *testing.T) {
		src := NewARNvramIndexSource(ARNvramIndexTypeUnknown, 0)
		data := map[source.ID]any{id(src): "raw"}
		if got := TranslateState(data); !reflect.DeepEqual(got, data) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("FlatWanValues", func(t *testing.T) {
		result := TranslateState(map[source.ID]any{
			id(ARNvramTypeLinkInternet):  "2",
			id(ARNvramTypeDualWanConfig): "lan usb",
		})
		if result[id(ARNvramTypeLinkInternet)] != common.ARConnectionStatusConnected {
			t.Errorf("link = %#v", result[id(ARNvramTypeLinkInternet)])
		}
		if got := result[id(ARNvramTypeDualWanConfig)]; !reflect.DeepEqual(got, []string{"lan", "usb"}) {
			t.Errorf("dual wan = %#v", got)
		}
	})
}

func TestAsItems(t *testing.T) {
	// Go addition: `_as_items` accepts every Go spelling of a request
	idx := NewARNvramIndexSource(ARNvramIndexTypeWanState, 0)
	tests := []struct {
		name    string
		request any
		want    int
	}{
		{"Single", ARNvramTypeMac, 1},
		{"Indexed", idx, 1},
		{"Items", []ARNvramItem{ARNvramTypeMac, idx}, 2},
		{"Types", []ARNvramType{ARNvramTypeMac, ARNvramTypeModel}, 2},
		{"DataSources", []source.DataSource{ARNvramTypeMac, idx}, 2},
		{"Collection", source.ARDataCollection{ARNvramTypeMac}, 1},
		{"None", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AsItems(tt.request); len(got) != tt.want {
				t.Errorf("got %d items, want %d", len(got), tt.want)
			}
		})
	}
}
