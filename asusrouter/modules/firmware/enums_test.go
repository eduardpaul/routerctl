package firmware

import "testing"

func TestValueRoundTrips(t *testing.T) {
	for _, m := range ARFirmwareTypeMembers {
		if got := ARFirmwareTypeFromValue(string(m)); got != m {
			t.Errorf("ARFirmwareType.%s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARFirmwareWebErrorMembers {
		if got := ARFirmwareWebErrorFromValue(int(m)); got != m {
			t.Errorf("ARFirmwareWebError.%s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARFirmwareWebFetchMembers {
		if got := ARFirmwareWebFetchFromValue(int(m)); got != m {
			t.Errorf("ARFirmwareWebFetch.%s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARFirmwareWebNotifyMembers {
		if got := ARFirmwareWebNotifyFromValue(int(m)); got != m {
			t.Errorf("ARFirmwareWebNotify.%s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARFirmwareWebUpgradeMembers {
		if got := ARFirmwareWebUpgradeFromValue(int(m)); got != m {
			t.Errorf("ARFirmwareWebUpgrade.%s: got %v", m.Name(), got)
		}
	}
}

func TestUnknownFallback(t *testing.T) {
	const raw = "zzz-not-a-real-enum-value"
	if ARFirmwareTypeFromValue(raw) != ARFirmwareTypeUnknown {
		t.Error("ARFirmwareType")
	}
	if ARFirmwareWebErrorFromValue(raw) != ARFirmwareWebErrorUnknown {
		t.Error("ARFirmwareWebError")
	}
	if ARFirmwareWebFetchFromValue(raw) != ARFirmwareWebFetchUnknown {
		t.Error("ARFirmwareWebFetch")
	}
	if ARFirmwareWebNotifyFromValue(raw) != ARFirmwareWebNotifyUnknown {
		t.Error("ARFirmwareWebNotify")
	}
	if ARFirmwareWebUpgradeFromValue(raw) != ARFirmwareWebUpgradeUnknown {
		t.Error("ARFirmwareWebUpgrade")
	}
}

func TestWebEnumCodesFromValue(t *testing.T) {
	cases := []struct {
		name     string
		got      any
		expected any
	}{
		{"error_0", ARFirmwareWebErrorFromValue("0"), ARFirmwareWebErrorNone},
		{"error_3", ARFirmwareWebErrorFromValue("3"), ARFirmwareWebErrorFwError},
		{"error_42", ARFirmwareWebErrorFromValue("42"), ARFirmwareWebErrorUnknown},
		{"fetch_0", ARFirmwareWebFetchFromValue("0"), ARFirmwareWebFetchActive},
		{"fetch_1", ARFirmwareWebFetchFromValue("1"), ARFirmwareWebFetchInactive},
		{"notify_2", ARFirmwareWebNotifyFromValue("2"), ARFirmwareWebNotifyForce},
		{"notify_empty", ARFirmwareWebNotifyFromValue(""), ARFirmwareWebNotifyUnknown},
		{"upgrade_-1", ARFirmwareWebUpgradeFromValue("-1"), ARFirmwareWebUpgradeInactive},
		{"upgrade_2", ARFirmwareWebUpgradeFromValue("2"), ARFirmwareWebUpgradeActive},
	}
	for _, c := range cases {
		if c.got != c.expected {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.expected)
		}
	}
}

func TestMerlinLike(t *testing.T) {
	for _, ft := range []ARFirmwareType{ARFirmwareTypeMerlin, ARFirmwareTypeGnuton} {
		if _, ok := ARFwMerlinLike[ft]; !ok {
			t.Errorf("%v not merlin-like", ft)
		}
	}
	if _, ok := ARFwMerlinLike[ARFirmwareTypeStock]; ok || len(ARFwMerlinLike) != 2 {
		t.Error("unexpected ARFwMerlinLike")
	}
}
