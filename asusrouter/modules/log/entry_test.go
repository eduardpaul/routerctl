package log

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Ported from tests/modules/log/test_entry.py.

func TestEntryFields(t *testing.T) {
	entry := ARLogEntry{TimestampStr: "Jul 21 15:08:01", Content: security.NewSensitiveText("up")}

	if entry.TimestampStr != "Jul 21 15:08:01" || entry.ContentValue() != "up" {
		t.Errorf("got %+v", entry)
	}
}

func TestEntryProgramName(t *testing.T) {
	tagged := ARLogEntry{
		TimestampStr: "Jul 21 15:08:01",
		Content:      security.NewSensitiveText("up"),
		Program:      &ARLogProgram{Name: "ntp"},
	}
	tagless := ARLogEntry{TimestampStr: "Jul 21 15:08:01", Content: security.NewSensitiveText("freeform")}

	if name := tagged.ProgramName(); name == nil || *name != "ntp" {
		t.Errorf("tagged: got %v", name)
	}
	if name := tagless.ProgramName(); name != nil {
		t.Errorf("tagless: got %q", *name)
	}
}

// test_message_is_not_stored_twice: the struct has no message field
// (checked by the compiler). test_frozen: Go structs are not frozen.

func TestEntryDefaultContent(t *testing.T) {
	// Python defaults content to EMPTY_CONTENT (UNSAFE, empty)
	if EmptyContent.Value != "" || EmptyContent.Level != security.ARSecurityLevelUnsafe {
		t.Errorf("got %+v", EmptyContent)
	}
}
