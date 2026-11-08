package identifiers

import (
	"strings"
	"testing"
)

func TestWiFiInterfaceParses(t *testing.T) {
	cases := []struct {
		value    any
		expected WiFiInterface
	}{
		{"0.2", WiFiInterface{0, 2}},
		{"0 2", WiFiInterface{0, 2}},
		{"wl0.2", WiFiInterface{0, 2}},
		{"1 1", WiFiInterface{1, 1}},
		{"2", WiFiInterface{2, 0}},
		{WiFiInterface{1, 3}, WiFiInterface{1, 3}},
		// Python int() details
		{"0.\t2", WiFiInterface{0, 2}},
		{"+1.-2", WiFiInterface{1, -2}},
		{"1_0.2", WiFiInterface{10, 2}},
		{3, WiFiInterface{3, 0}},
	}
	for _, c := range cases {
		got, err := WiFiInterfaceFromValue(c.value)
		if err != nil || got != c.expected {
			t.Errorf("WiFiInterfaceFromValue(%#v) = %v, %v", c.value, got, err)
		}
	}
}

func TestWiFiInterfaceInvalid(t *testing.T) {
	for _, v := range []string{"", "a.b", "0.1.2", "wl", "wl 1", "0x1"} {
		if _, err := WiFiInterfaceFromValue(v); err == nil || !strings.Contains(err.Error(), "Invalid WiFi interface") {
			t.Errorf("WiFiInterfaceFromValue(%q) error = %v", v, err)
		}
	}
}

func TestWiFiInterfaceFromValueSafe(t *testing.T) {
	if got := WiFiInterfaceFromValueSafe("0 2"); got == nil || *got != (WiFiInterface{0, 2}) {
		t.Fatal("valid")
	}
	for _, v := range []any{nil, "", "bad"} {
		if WiFiInterfaceFromValueSafe(v) != nil {
			t.Errorf("safe(%#v)", v)
		}
	}
}

func TestWiFiInterfaceStr(t *testing.T) {
	iface, _ := WiFiInterfaceFromValue("0 2")
	if iface.String() != "0.2" {
		t.Fatal(iface.String())
	}
}

func TestWiFiInterfaceHashable(t *testing.T) {
	if !(map[WiFiInterface]bool{{0, 2}: true})[WiFiInterface{0, 2}] {
		t.Fatal("hashable")
	}
}
