package ddns

import "testing"

func TestEnums(t *testing.T) {
	for _, m := range ARDdnsCommandMembers {
		if ARDdnsCommandFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	for _, m := range ARDdnsFieldMembers {
		if ARDdnsFieldFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	for _, m := range ARDdnsServerMembers {
		if ARDdnsServerFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	// An empty-string value (ARDdnsStatus.NONE) is a sentinel: blank input
	// resolves to UNKNOWN
	for _, m := range ARDdnsStatusMembers {
		if m != "" && ARDdnsStatusFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	const bogus = "zzz-not-a-real-enum-value"
	if ARDdnsCommandFromValue(bogus) != ARDdnsCommandUnknown || ARDdnsFieldFromValue(bogus) != ARDdnsFieldUnknown ||
		ARDdnsServerFromValue(bogus) != ARDdnsServerUnknown || ARDdnsStatusFromValue(bogus) != ARDdnsStatusUnknown {
		t.Error("unknown fallback")
	}
	if ARDdnsStatusNone != "" || ARDdnsStatusFromValue("") != ARDdnsStatusUnknown {
		t.Error("blank status is unknown")
	}
}
