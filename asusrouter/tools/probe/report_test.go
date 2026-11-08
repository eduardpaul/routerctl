package probe

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const testMAC = "AA:BB:CC:DD:EE:FF"

func mac(t *testing.T) identifiers.MacAddress {
	m, err := identifiers.MacAddressFromValue(testMAC)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRenderValue(t *testing.T) {
	if RenderValue(42, security.ARSecurityLevelStrict) != "42" {
		t.Error("plain value")
	}
	if got := RenderValue("client "+testMAC+" at 192.168.1.5", security.ARSecurityLevelSanitized); got != "client "+RedactedMAC+" at "+RedactedIP {
		t.Errorf("scrubbed: %q", got)
	}
	if text := "client " + testMAC; RenderValue(text, security.ARSecurityLevelReasonable) != text {
		t.Error("text not kept")
	}
	// A stable stand-in, so the same client stays recognizable
	rendered := RenderValue(mac(t), security.ARSecurityLevelSanitized)
	if rendered == testMAC || rendered == RedactedMAC || rendered == security.RedactedStr ||
		rendered != RenderValue(mac(t), security.ARSecurityLevelSanitized) {
		t.Errorf("masked: %q", rendered)
	}
	if got := RenderValue(mac(t), security.ARSecurityLevelUnsafe); got != strings.ToLower(testMAC) {
		t.Errorf("revealed: %q", got)
	}
	value := security.Sensitive{Value: "secret", Level: security.ARSecurityLevelUnsafe}
	if RenderValue(value, security.ARSecurityLevelStrict) != security.RedactedStr {
		t.Error("not redacted")
	}
}

func TestBuildSection(t *testing.T) {
	section := BuildSection("buffer", []Row{{"entries", 12}, {"newest", "from " + testMAC}}, security.ARSecurityLevelSanitized)
	want := ARProbeSection{"buffer", [][2]string{{"entries", "12"}, {"newest", "from " + RedactedMAC}}}
	if !reflect.DeepEqual(section, want) {
		t.Fatal(section)
	}
	if len(BuildSection("empty", nil, security.ARSecurityLevelStrict).Rows) != 0 {
		t.Fatal("rows")
	}
}

func TestARProbeReportDefaults(t *testing.T) {
	report := ARProbeReport{Source: "ARLogSource", Level: security.ARSecurityLevelSanitized}
	if len(report.Sections) != 0 {
		t.Fatal("sections")
	}
}
