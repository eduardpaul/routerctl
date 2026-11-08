package parentalcontrol

import "testing"

func TestEnums(t *testing.T) {
	for _, m := range ARParentalControlCommandMembers {
		if ARParentalControlCommandFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	for _, m := range ARParentalControlTypeMembers {
		if ARParentalControlTypeFromValue(int(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	for _, m := range ARWeekdayMembers {
		if ARWeekdayFromValue(int(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	for _, m := range ARParentalControlScheduleModeMembers {
		if ARParentalControlScheduleModeFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	for _, m := range ARParentalControlFieldMembers {
		if ARParentalControlFieldFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	const bogus = "zzz-not-a-real-enum-value"
	if ARParentalControlCommandFromValue(bogus) != ARParentalControlCommandUnknown ||
		ARParentalControlTypeFromValue(bogus) != ARParentalControlTypeUnknown ||
		ARWeekdayFromValue(bogus) != ARWeekdayUnknown ||
		ARParentalControlScheduleModeFromValue(bogus) != ARParentalControlScheduleModeUnknown ||
		ARParentalControlFieldFromValue(bogus) != ARParentalControlFieldUnknown {
		t.Error("unknown fallback")
	}
}
