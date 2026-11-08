package query

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/translate"
)

var dimMac = mustMac("aa:bb:cc:00:00:01")

func dimEvent(data map[log.AREventKey]any) *ARLogEvent {
	return newEvent(eventOpts{
		program:     log.ARProgramNtp,
		programName: strPtr("ntp"),
		eventType:   translate.AREventNtpStartUpdate,
		raw:         "",
		data:        data,
	})
}

func TestCoerce(t *testing.T) {
	t.Run("test_mac_normalizes_format", func(t *testing.T) {
		if !pyEqual(Coerce(log.AREventKeyClientMac, "aa-bb-cc-00-00-01"), dimMac) {
			t.Fatal("dash MAC did not coerce to the MacAddress")
		}
	})
	t.Run("test_program_from_string", func(t *testing.T) {
		if got := Coerce(log.AREventKeyProgram, "ntp"); got != log.ARProgramNtp {
			t.Fatalf("got %#v", got)
		}
	})
	t.Run("test_identity_for_plain_dimension", func(t *testing.T) {
		if got := Coerce(log.AREventKeyCaller, "udhcpc"); got != "udhcpc" {
			t.Fatalf("got %#v", got)
		}
	})
}

func TestExtractFacets(t *testing.T) {
	type facet struct {
		axis  log.AREventKey
		value any
	}
	tests := []struct {
		name string
		data map[log.AREventKey]any
		want []facet
	}{
		{"test_scalar_field", map[log.AREventKey]any{log.AREventKeyClientMac: &dimMac},
			[]facet{{log.AREventKeyClientMac, dimMac}}},
		{"test_non_facetable_field_ignored", map[log.AREventKey]any{log.AREventKeyVersion: "1.2.3"}, nil},
		{"test_collection_expands_to_axis",
			map[log.AREventKey]any{log.AREventKeyServices: []string{"restart_wan", "restart_firewall"}},
			[]facet{{log.AREventKeyService, "restart_wan"}, {log.AREventKeyService, "restart_firewall"}}},
		{"test_string_not_flattened", map[log.AREventKey]any{log.AREventKeyInterface: "br0"},
			[]facet{{log.AREventKeyInterface, "br0"}}},
		{"test_none_skipped", map[log.AREventKey]any{log.AREventKeyClientMac: nil}, nil},
		{"test_shared_axis_merges",
			map[log.AREventKey]any{
				log.AREventKeyService:  "restart_wan",
				log.AREventKeyServices: []string{"restart_firewall"},
			},
			[]facet{{log.AREventKeyService, "restart_wan"}, {log.AREventKeyService, "restart_firewall"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facets := ExtractFacets(tt.data)
			if facets.Len() != len(tt.want) {
				t.Fatalf("got %d facets %v, want %d", facets.Len(), facets.Items(), len(tt.want))
			}
			for _, f := range tt.want {
				if !facets.Has(f.axis, f.value) {
					t.Errorf("missing facet (%s, %v)", f.axis, f.value)
				}
			}
		})
	}
}

func TestHas(t *testing.T) {
	t.Run("test_program_dimension", func(t *testing.T) {
		if !Has(dimEvent(nil), log.AREventKeyProgram, log.ARProgramNtp) {
			t.Error("NTP did not match")
		}
		if Has(dimEvent(nil), log.AREventKeyProgram, log.ARProgramKernel) {
			t.Error("KERNEL matched")
		}
	})
	t.Run("test_event_type_dimension", func(t *testing.T) {
		event := dimEvent(nil)
		if !Has(event, log.AREventKeyEventType, translate.AREventNtpStartUpdate) {
			t.Error("START_UPDATE did not match")
		}
		if Has(event, log.AREventKeyEventType, translate.AREventNtpUnknown) {
			t.Error("UNKNOWN matched")
		}
	})
	t.Run("test_facet_axis", func(t *testing.T) {
		event := dimEvent(map[log.AREventKey]any{log.AREventKeyClientMac: &dimMac})
		if !Has(event, log.AREventKeyClientMac, dimMac) {
			t.Error("facet did not match")
		}
	})
	t.Run("test_absent_axis", func(t *testing.T) {
		if Has(dimEvent(nil), log.AREventKeyClientMac, dimMac) {
			t.Error("absent axis matched")
		}
	})
	t.Run("test_value_on_other_axis", func(t *testing.T) {
		event := dimEvent(map[log.AREventKey]any{log.AREventKeyNodeMac: &dimMac})
		if Has(event, log.AREventKeyClientMac, dimMac) {
			t.Error("value on another axis matched")
		}
	})
	t.Run("test_multi_valued_axis", func(t *testing.T) {
		event := dimEvent(map[log.AREventKey]any{
			log.AREventKeyServices: []common.ARService{common.ARServiceWanRestart, common.ARServiceFirewallRestart},
		})
		if !Has(event, log.AREventKeyService, common.ARServiceWanRestart) ||
			!Has(event, log.AREventKeyService, common.ARServiceFirewallRestart) {
			t.Error("a service did not match")
		}
		if Has(event, log.AREventKeyService, common.ARServiceReboot) {
			t.Error("REBOOT matched")
		}
	})
}

func TestResolve(t *testing.T) {
	services := dimEvent(map[log.AREventKey]any{
		log.AREventKeyServices: []common.ARService{common.ARServiceWanRestart, common.ARServiceFirewallRestart},
	})
	tests := []struct {
		name      string
		event     *ARLogEvent
		dimension log.AREventKey
		want      []any
	}{
		{"test_program_dimension", dimEvent(nil), log.AREventKeyProgram, []any{log.ARProgramNtp}},
		{"test_program_name_dimension", dimEvent(nil), log.AREventKeyProgramName, []any{"ntp"}},
		{"test_event_type_dimension", dimEvent(nil), log.AREventKeyEventType, []any{translate.AREventNtpStartUpdate}},
		{"test_facet_axis", dimEvent(map[log.AREventKey]any{log.AREventKeyClientMac: &dimMac}),
			log.AREventKeyClientMac, []any{dimMac}},
		{"test_absent_axis_is_empty", dimEvent(nil), log.AREventKeyClientMac, nil},
		{"test_multi_valued_axis", services, log.AREventKeyService,
			[]any{common.ARServiceWanRestart, common.ARServiceFirewallRestart}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.event, tt.dimension)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for _, w := range tt.want {
				if !containsKey(got, w) {
					t.Errorf("missing %v in %v", w, got)
				}
			}
		})
	}
}

func TestQueryable(t *testing.T) {
	in := func(k log.AREventKey) bool { _, ok := QUERYABLE[k]; return ok }
	t.Run("test_holds_envelope_and_facet_axes", func(t *testing.T) {
		for _, k := range []log.AREventKey{log.AREventKeyProgram, log.AREventKeyEventType, log.AREventKeyClientMac} {
			if !in(k) {
				t.Errorf("%s not queryable", k)
			}
		}
	})
	t.Run("test_omits_plain_data_fields", func(t *testing.T) {
		if in(log.AREventKeyVersion) {
			t.Error("VERSION is queryable")
		}
	})
	t.Run("test_lists_the_axis_not_the_source_field", func(t *testing.T) {
		if !in(log.AREventKeyService) || in(log.AREventKeyServices) {
			t.Error("SERVICE/SERVICES listing wrong")
		}
	})
}
