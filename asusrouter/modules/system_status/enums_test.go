package systemstatus

import "testing"

func TestEnums(t *testing.T) {
	for _, member := range ARSystemTypeMembers {
		if ARSystemTypeFromValue(string(member)) != member {
			t.Errorf("%s does not round-trip", member.Name())
		}
	}
	if ARSystemTypeFromValue("zzz-not-a-real-enum-value") != ARSystemTypeUnknown {
		t.Error("unknown fallback")
	}
}
