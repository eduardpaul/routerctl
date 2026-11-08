package traffic

import "testing"

func TestEnums(t *testing.T) {
	for _, member := range ARTrafficTypeMembers {
		if ARTrafficTypeFromValue(string(member)) != member {
			t.Errorf("%s does not round-trip", member.Name())
		}
	}
	if ARTrafficTypeFromValue("zzz-not-a-real-enum-value") != ARTrafficTypeUnknown {
		t.Error("unknown fallback")
	}
}
