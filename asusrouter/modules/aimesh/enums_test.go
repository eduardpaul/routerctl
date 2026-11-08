package aimesh

import "testing"

// TestValueRoundTrips mirrors test_value_round_trips: every member
// resolves from its own stored value.
func TestValueRoundTrips(t *testing.T) {
	for _, m := range ARAiMeshCapabilityMembers {
		if got := ARAiMeshCapabilityFromValue(string(m)); got != m {
			t.Errorf("ARAiMeshCapability %q: got %q", m, got)
		}
	}
	for _, m := range ARAiMeshDirectionMembers {
		if got := ARAiMeshDirectionFromValue(string(m)); got != m {
			t.Errorf("ARAiMeshDirection %q: got %q", m, got)
		}
	}
	for _, m := range ARAiMeshFeatureMembers {
		if got := ARAiMeshFeatureFromValue(string(m)); got != m {
			t.Errorf("ARAiMeshFeature %q: got %q", m, got)
		}
	}
	for _, m := range ARAiMeshMediumMembers {
		if got := ARAiMeshMediumFromValue(string(m)); got != m {
			t.Errorf("ARAiMeshMedium %q: got %q", m, got)
		}
	}
	for _, m := range ARAiMeshRoleMembers {
		if got := ARAiMeshRoleFromValue(string(m)); got != m {
			t.Errorf("ARAiMeshRole %q: got %q", m, got)
		}
	}
}

// TestUnknownFallback mirrors test_unknown_fallback.
func TestUnknownFallback(t *testing.T) {
	const bad = "zzz-not-a-real-enum-value"
	checks := []struct {
		name string
		ok   bool
	}{
		{"ARAiMeshCapability", ARAiMeshCapabilityFromValue(bad) == ARAiMeshCapabilityUnknown},
		{"ARAiMeshDirection", ARAiMeshDirectionFromValue(bad) == ARAiMeshDirectionUnknown},
		{"ARAiMeshFeature", ARAiMeshFeatureFromValue(bad) == ARAiMeshFeatureUnknown},
		{"ARAiMeshMedium", ARAiMeshMediumFromValue(bad) == ARAiMeshMediumUnknown},
		{"ARAiMeshRole", ARAiMeshRoleFromValue(bad) == ARAiMeshRoleUnknown},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("%s: no UNKNOWN fallback", c.name)
		}
	}
}
