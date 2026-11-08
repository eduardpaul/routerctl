package query

import (
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/translate"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

func eventEntry(name *string, content string) *log.ARLogEntry {
	var program *log.ARLogProgram
	if name != nil {
		program = &log.ARLogProgram{Name: *name}
	}
	return &log.ARLogEntry{
		TimestampStr: "Jul 22 08:43:21",
		Program:      program,
		Content:      security.NewSensitiveText(content),
	}
}

func TestBuildEvent(t *testing.T) {
	t.Run("test_metadata_lifted", func(t *testing.T) {
		event := BuildEvent(eventEntry(strPtr("ntp"), "start NTP update"))
		if event.Program != log.ARProgramNtp {
			t.Errorf("program %v", event.Program)
		}
		if event.ProgramName == nil || *event.ProgramName != "ntp" {
			t.Errorf("program_name %v", event.ProgramName)
		}
		if event.EventType != translate.AREventNtpStartUpdate {
			t.Errorf("event_type %v", event.EventType)
		}
		if event.Raw != "start NTP update" {
			t.Errorf("raw %#v", event.Raw)
		}
		if len(event.Data) != 0 || event.Facets().Len() != 0 {
			t.Errorf("data %v facets %v", event.Data, event.Facets().Items())
		}
	})
	// test_hashable: Python checks the envelope hashes despite its dict
	// payload; Go events are pointers and need no hash. Port the intent:
	// two builds of one entry carry the same envelope values.
	t.Run("test_hashable", func(t *testing.T) {
		entry := eventEntry(strPtr("rc_service"), "udhcpc_wan 1657:notify_rc restart_wan")
		a, b := BuildEvent(entry), BuildEvent(entry)
		if len(a.Data) == 0 {
			t.Fatal("no data")
		}
		if a.EventType != b.EventType || a.Program != b.Program || a.RawText() != b.RawText() {
			t.Errorf("builds differ: %+v vs %+v", a, b)
		}
	})
	t.Run("test_data_excludes_metadata", func(t *testing.T) {
		event := BuildEvent(eventEntry(strPtr("rc_service"), "udhcpc_wan 1657:notify_rc restart_wan;restart_firewall"))
		if event.EventType != translate.AREventRcServiceNotify {
			t.Errorf("event_type %v", event.EventType)
		}
		if _, ok := event.Data[log.AREventKeyEventType]; ok {
			t.Error("EVENT_TYPE in data")
		}
		if _, ok := event.Data[log.AREventKeyRaw]; ok {
			t.Error("RAW in data")
		}
		if event.Data[log.AREventKeyCaller] != "udhcpc_wan" {
			t.Errorf("caller %v", event.Data[log.AREventKeyCaller])
		}
	})
	t.Run("test_facets_derived", func(t *testing.T) {
		event := BuildEvent(eventEntry(strPtr("rc_service"), "udhcpc_wan 1657:notify_rc restart_wan;restart_firewall"))
		facets := event.Facets()
		if facets.Len() != 3 ||
			!facets.Has(log.AREventKeyCaller, "udhcpc_wan") ||
			!facets.Has(log.AREventKeyService, common.ARServiceWanRestart) ||
			!facets.Has(log.AREventKeyService, common.ARServiceFirewallRestart) {
			t.Fatalf("facets %v", facets.Items())
		}
	})
	t.Run("test_tagless_is_unknown", func(t *testing.T) {
		event := BuildEvent(eventEntry(nil, "bare line"))
		if event.Program != log.ARProgramUnknown || event.ProgramName != nil ||
			event.EventType != translate.AREventUnknownUnknown || len(event.Data) != 0 {
			t.Fatalf("got %+v", event)
		}
	})
	t.Run("test_untyped_program_keeps_its_tag", func(t *testing.T) {
		event := BuildEvent(eventEntry(strPtr("ARK"), "ARK is blocked, reason=3!!"))
		if event.Program != log.ARProgramUnknown || event.ProgramName == nil || *event.ProgramName != "ARK" {
			t.Fatalf("got %+v", event)
		}
	})
}

func TestBuildEvents(t *testing.T) {
	t.Run("test_parses_and_resolves", func(t *testing.T) {
		raw := "Jul 22 08:43:21 ntp: start NTP update\n" +
			"Jul 22 08:43:22 rc_service: cfg 1:notify_rc restart_wan\n"
		anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		events := BuildEvents(raw, &anchor)
		if len(events) != 2 || events[0].Program != log.ARProgramNtp || events[1].Program != log.ARProgramRcService {
			t.Fatalf("got %v", events)
		}
		for _, e := range events {
			if e.Timestamp == nil {
				t.Error("unresolved timestamp")
			}
		}
	})
	t.Run("test_empty", func(t *testing.T) {
		if got := BuildEvents("", nil); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
}

func TestRawText(t *testing.T) {
	t.Run("test_classified_raw_reaches_its_value", func(t *testing.T) {
		event := BuildEvent(eventEntry(strPtr("dnsmasq"), "reading /etc/hosts"))
		raw, ok := event.Raw.(security.SensitiveText)
		if !ok {
			t.Fatalf("raw %#v is not a SensitiveText", event.Raw)
		}
		// Python str() redacts: Go spells that PyStr
		if raw.PyStr() == "reading /etc/hosts" {
			t.Error("str() of the raw is not redacted")
		}
		if event.RawText() != "reading /etc/hosts" {
			t.Errorf("raw_text %q", event.RawText())
		}
	})
	t.Run("test_plain_raw_passes_through", func(t *testing.T) {
		event := BuildEvent(eventEntry(strPtr("ntp"), "start NTP update"))
		if event.Raw != "start NTP update" || event.RawText() != "start NTP update" {
			t.Fatalf("raw %#v", event.Raw)
		}
	})
}
