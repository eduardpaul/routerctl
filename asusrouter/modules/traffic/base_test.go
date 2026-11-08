package traffic

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const (
	target = "AA:BB:CC:00:00:01"
	other  = "AA:BB:CC:00:00:02"
)

func TestTargetCoercion(t *testing.T) {
	m := identifiers.MacAddressFromValueSafe(target)
	tests := []struct {
		name  string
		value any
		want  *identifiers.MacAddress
	}{
		{"str", target, m},
		{"macaddress", *m, m},
		{"none", nil, nil},
		{"invalid", "not-a-mac", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewARTrafficSource(nil, tt.value).Target(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinkCoercion(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  ARTrafficLink
	}{
		{"none", nil, nil},
		{"type", ARTrafficTypeWired, ARTrafficTypeWired},
		{"band", wifi.ARWiFiBandBand5g1, wifi.ARWiFiBandBand5g1},
		{"str-type", "wired", ARTrafficTypeWired},
		{"str-backhaul", "backhaul", ARTrafficTypeBackhaul},
		{"str-band", "5g1", wifi.ARWiFiBandBand5g1},
		{"invalid", "garbage", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewARTrafficSource(tt.value, nil).Link(); got != tt.want {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestReprAndEquality(t *testing.T) {
	s := NewARTrafficSource(ARTrafficTypeWired, target)
	text := source.Repr(s)
	if !strings.HasPrefix(text, "<ARTrafficSource ") || !strings.Contains(text, s.Target().String()) || !strings.Contains(text, "wired") {
		t.Errorf("repr %q", text)
	}

	if source.IDOf(s) != source.IDOf(NewARTrafficSource(ARTrafficTypeWired, target)) {
		t.Error("equal by type, target and link")
	}
	for name, o := range map[string]*ARTrafficSource{
		"link":   NewARTrafficSource(ARTrafficTypeWan, target),
		"target": NewARTrafficSource(ARTrafficTypeWired, other),
	} {
		if source.IDOf(s) == source.IDOf(o) {
			t.Errorf("%s: must differ", name)
		}
	}
}

type sub struct{ ARTrafficSource }

func (*sub) SourceName() string { return "_Sub" }

func TestSubclassNotEqual(t *testing.T) {
	s := &sub{}
	s.Init(nil, target)
	if source.IDOf(NewARTrafficSource(nil, target)) == source.IDOf(s) {
		t.Error("a subclass instance is never equal to the base")
	}
}
