package translate

// Tests for the program-based log entry dispatch.

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// entry builds an entry with the given program token ("" for none) and
// content.
func entry(name string, content string) *log.ARLogEntry {
	var program *log.ARLogProgram
	if name != "" {
		program = &log.ARLogProgram{Name: name}
	}

	return &log.ARLogEntry{
		TimestampStr: "Jul 22 08:43:21",
		Program:      program,
		Content:      security.NewSensitiveText(content),
	}
}

func TestResolveProgram(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  log.ARProgram
	}{
		{"known_program", "kernel", log.ARProgramKernel},
		{"aliased_program", "amas_lib", log.ARProgramRcService},
		{"indexed_program", "vpnclient5", log.ARProgramVpnClient},
		{"unmapped_program", "nobody", log.ARProgramUnknown},
		{"no_program", "", log.ARProgramUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveProgram(entry(tt.token, "")); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslateEntry(t *testing.T) {
	t.Run("rc_service", func(t *testing.T) {
		e := entry("rc_service", "service 2011:notify_rc restart_firewall")
		assertEvent(t, TranslateEntry(e, nil), ev{
			log.AREventKeyEventType: AREventRcServiceNotify,
			log.AREventKeyRaw:       e.Content.Value,
			log.AREventKeyCaller:    "service",
			log.AREventKeyPid:       2011,
			log.AREventKeyServices:  []common.ARService{common.ARServiceFirewallRestart},
			log.AREventKeyActions:   []string{"restart_firewall"},
		})
	})
	t.Run("kernel", func(t *testing.T) {
		e := entry("kernel", "br0: topology change detected, propagating")
		eq(t, TranslateEntry(e, nil), log.AREventKeyEventType, AREventKernelBridgeTopologyChange)
	})
	t.Run("alias", func(t *testing.T) {
		// The httpd dispatcher always returns a dict (UNKNOWN if unmatched)
		if TranslateEntry(entry("HTTPD", "HTTPD: some future httpd message"), nil) == nil {
			t.Fatal("got nil")
		}
	})
	t.Run("unmapped_program", func(t *testing.T) {
		assertEvent(t, TranslateEntry(entry("dnsmasq", "reading /etc/hosts"), nil), ev{
			log.AREventKeyEventType: AREventUnknownUnknown,
			log.AREventKeyRaw:       unread("reading /etc/hosts"),
		})
	})
	t.Run("indexed_unmapped", func(t *testing.T) {
		eq(t, TranslateEntry(entry("vpnclient5", "Initialization"), nil),
			log.AREventKeyEventType, AREventUnknownUnknown)
	})
	t.Run("no_program", func(t *testing.T) {
		assertEvent(t, TranslateEntry(entry("", "syslogd exiting"), nil), ev{
			log.AREventKeyEventType: AREventUnknownUnknown,
			log.AREventKeyRaw:       unread("syslogd exiting"),
		})
	})
	t.Run("program_provided", func(t *testing.T) {
		e := entry("kernel", "br0: topology change detected")
		kernel := log.ARProgramKernel
		assertEvent(t, TranslateEntry(e, &kernel), TranslateEntry(e, nil))
	})
	t.Run("program_provided_overrides_token", func(t *testing.T) {
		e := entry("kernel", "reading /etc/hosts")
		dnsmasq := log.ARProgramDnsmasq
		assertEvent(t, TranslateEntry(e, &dnsmasq), ev{
			log.AREventKeyEventType: AREventUnknownUnknown,
			log.AREventKeyRaw:       unread("reading /etc/hosts"),
		})
	})
}

func TestTranslateEntries(t *testing.T) {
	t.Run("maps_all", func(t *testing.T) {
		events := TranslateEntries([]*log.ARLogEntry{
			entry("rc_service", "service 2011:notify_rc restart_firewall"),
			entry("dnsmasq", "reading /etc/hosts"),
			entry("", "syslogd exiting"),
			entry("kernel", "br0: topology change detected"),
		})
		if len(events) != 4 {
			t.Fatalf("got %d events", len(events))
		}
		eq(t, events[0], log.AREventKeyEventType, AREventRcServiceNotify)
		eq(t, events[1], log.AREventKeyEventType, AREventUnknownUnknown)
		eq(t, events[2], log.AREventKeyEventType, AREventUnknownUnknown)
		eq(t, events[3], log.AREventKeyEventType, AREventKernelBridgeTopologyChange)
	})
	t.Run("empty", func(t *testing.T) {
		if got := TranslateEntries(nil); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
}

func TestTranslationCache(t *testing.T) {
	t.Run("repeat_is_cached", func(t *testing.T) {
		// Python reads lru_cache hits/misses; Go checks the one cached item
		content := "br0: port 7(eth9) entering forwarding state"
		contentMu.Lock()
		contentCache = map[contentKey]map[log.AREventKey]any{}
		contentMu.Unlock()

		TranslateEntry(entry("kernel", content), nil)
		TranslateEntry(entry("kernel", content), nil)

		contentMu.Lock()
		defer contentMu.Unlock()
		if len(contentCache) != 1 {
			t.Fatalf("cache holds %d items, want 1", len(contentCache))
		}
		if _, ok := contentCache[contentKey{log.ARProgramKernel, content}]; !ok {
			t.Fatal("message not cached")
		}
	})
	t.Run("caller_owns_the_result", func(t *testing.T) {
		content := "br0: topology change detected"
		first := TranslateEntry(entry("kernel", content), nil)
		for k := range first {
			delete(first, k)
		}

		assertEvent(t, TranslateEntry(entry("kernel", content), nil), ev{
			log.AREventKeyEventType: AREventKernelBridgeTopologyChange,
			log.AREventKeyRaw:       content,
			log.AREventKeyBridge:    "br0",
		})
	})
	t.Run("program_is_part_of_the_key", func(t *testing.T) {
		content := "br0: topology change detected"
		if reflect.DeepEqual(
			TranslateEntry(entry("kernel", content), nil),
			TranslateEntry(entry("dnsmasq", content), nil),
		) {
			t.Fatal("the two programs translated alike")
		}
	})
}
