package nvram

import (
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
)

func TestARNvramType(t *testing.T) {
	t.Run("ValueRoundTrip", func(t *testing.T) {
		for _, m := range ARNvramTypeMembers {
			if got := ARNvramTypeFromValue(string(m)); got != m {
				t.Errorf("%s: got %v", m.Name(), got)
			}
		}
	})
	t.Run("AsHook", func(t *testing.T) {
		for _, m := range ARNvramTypeMembers {
			want := endpoint.ARHookCall{Hook: endpoint.ARHookNvramGet, Args: string(m)}
			if got := m.AsHook(); got != want {
				t.Errorf("%s: got %v, want %v", m.Name(), got, want)
			}
		}
	})
	t.Run("AsHookExample", func(t *testing.T) {
		want := endpoint.ARHookCall{Hook: endpoint.ARHookNvramGet, Args: "label_mac"}
		if got := ARNvramTypeMac.AsHook(); got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("UnknownFallback", func(t *testing.T) {
		if got := ARNvramTypeFromValue("no-such-nvram-key"); got != ARNvramTypeUnknown {
			t.Errorf("got %v, want UNKNOWN", got)
		}
	})
}

func TestARNvramIndexType(t *testing.T) {
	t.Run("ValueRoundTrip", func(t *testing.T) {
		for _, m := range ARNvramIndexTypeMembers {
			if got := ARNvramIndexTypeFromValue(string(m)); got != m {
				t.Errorf("%s: got %v", m.Name(), got)
			}
		}
	})
	t.Run("KeyResolvesTemplate", func(t *testing.T) {
		for _, m := range ARNvramIndexTypeMembers {
			// Python `member.value.format(0)` (one `{}` per template)
			want := strings.Replace(string(m), "{}", "0", 1)
			if got := m.Key(0); got != want {
				t.Errorf("%s: got %q, want %q", m.Name(), got, want)
			}
		}
	})
	t.Run("KeyExample", func(t *testing.T) {
		if got := ARNvramIndexTypeApEnable.Key("g1"); got != "apg1_enable" {
			t.Errorf("got %q, want apg1_enable", got)
		}
	})
	t.Run("UnknownFallback", func(t *testing.T) {
		if got := ARNvramIndexTypeFromValue("no-such-template"); got != ARNvramIndexTypeUnknown {
			t.Errorf("got %v, want UNKNOWN", got)
		}
	})
}
