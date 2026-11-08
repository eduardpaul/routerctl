package targets

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const raw = "<Cloudflare>1.1.1.1<Cloudflare>1.0.0.1<Google>8.8.8.8"

func ip(value string) identifiers.IpAddress { return *identifiers.IpAddressFromValueSafe(value) }

func target(name, value string) ARPingTarget { return ARPingTarget{Name: name, IP: ip(value)} }

type listCalls struct {
	n     int
	force []bool
}

// listFetch is a FetchDataMulti returning the given dns_ping_list.
func listFetch(calls *listCalls, value any) registry.FetchDataMultiFunc {
	return func(_ context.Context, srcs []source.DataSource, kw registry.Kwargs) (map[source.ID]any, error) {
		calls.n++
		calls.force = append(calls.force, kw.Bool("force"))
		if value == nil || len(srcs) != 1 || srcs[0] != source.DataSource(nvram.ARNvramTypeDnsPingList) {
			return nil, nil
		}
		return map[source.ID]any{source.IDOf(nvram.ARNvramTypeDnsPingList): value}, nil
	}
}

func TestParseTargets(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []ARPingTarget
	}{
		{"parses_entries", raw, []ARPingTarget{target("Cloudflare", "1.1.1.1"), target("Cloudflare", "1.0.0.1"), target("Google", "8.8.8.8")}},
		{"decodes_html_entities", "&#60Cloudflare&#621.1.1.1&#60Google&#628.8.8.8", []ARPingTarget{target("Cloudflare", "1.1.1.1"), target("Google", "8.8.8.8")}},
		{"empty", "", []ARPingTarget{}},
		{"skips_malformed", "<NoIp><Bad>not-an-ip<Ok>1.1.1.1", []ARPingTarget{target("Ok", "1.1.1.1")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTargets(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSource(t *testing.T) {
	if source.IDOf(&ARPingTargetsSource{}) != source.IDOf(ARPingTargetsSourceUniversal) {
		t.Error("equal by type")
	}
	if e := registry.SourceByName("ARPingTargetsSource"); e == nil || e.Fetch == nil || e.Translate == nil {
		t.Error("not registered")
	}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()
	isEmpty := func(v any) bool { m, ok := v.(map[string]any); return ok && len(m) == 0 }

	if got, _ := fetchState(ctx, &registry.Context{}, ARPingTargetsSourceUniversal); !isEmpty(got) {
		t.Errorf("no_data_callback: %v", got)
	}
	var calls listCalls
	if got, _ := fetchState(ctx, &registry.Context{FetchDataMulti: listFetch(&calls, raw)}, ARPingTargetsSourceUniversal); got != raw {
		t.Errorf("returns_raw: %v", got)
	}
	if got, _ := fetchState(ctx, &registry.Context{FetchDataMulti: listFetch(&calls, nil)}, ARPingTargetsSourceUniversal); !isEmpty(got) {
		t.Errorf("non_dict_values: %v", got)
	}
}

func TestTranslateState(t *testing.T) {
	if got := translateState(raw, nil); !reflect.DeepEqual(got, parseTargets(raw)) {
		t.Errorf("parses_string: %v", got)
	}
	if got := translateState(nil, nil).([]ARPingTarget); len(got) != 0 {
		t.Errorf("non_string: %v", got)
	}
}
