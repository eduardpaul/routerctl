package routerkit

import (
	"fmt"
	"time"
)

// Support is how far a device supports a part of the model.
type Support uint8

// Support levels.
const (
	// Unsupported: the part is always nil and never an error
	Unsupported Support = iota
	// Partial: some fields are never filled (PartSupport.Missing)
	Partial
	// Supported: every field can be filled
	Supported
)

var supportNames = []string{"unsupported", "partial", "supported"}

// String implements fmt.Stringer.
func (s Support) String() string {
	if int(s) < len(supportNames) {
		return supportNames[s]
	}
	return fmt.Sprintf("support(%d)", uint8(s))
}

// MarshalText implements encoding.TextMarshaler.
func (s Support) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (s *Support) UnmarshalText(b []byte) error {
	for i, n := range supportNames {
		if n == string(b) {
			*s = Support(i)
			return nil
		}
	}
	return fmt.Errorf("unknown support level %q", b)
}

// PartSupport is the support of one part.
type PartSupport struct {
	Level Support `json:"level"`
	// Missing lists the JSON field names of the part (of each element
	// for a list part) that this device never sets
	Missing []string `json:"missing,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// Feature is an optional device feature ("mesh", "pon", ...).
type Feature string

// Known features.
const (
	FeatureMesh       Feature = "mesh"
	FeaturePON        Feature = "pon"
	FeatureDSL        Feature = "dsl"
	FeatureGuestWiFi  Feature = "guest_wifi"
	FeatureMLO        Feature = "mlo"
	FeatureController Feature = "controller"
)

// Capabilities is what a device supports: data-driven feature detection
// (support depends on the model and firmware, not on the Go type).
type Capabilities struct {
	Parts    map[Part]PartSupport `json:"parts"`
	Features []Feature            `json:"features,omitempty"`
	Actions  []ActionSpec         `json:"actions,omitempty"`
	Sources  []SourceSpec         `json:"sources,omitempty"`
	// MinInterval is the shortest spacing between polls the device
	// tolerates (a scheduling hint for the poller)
	MinInterval time.Duration `json:"min_interval,omitempty"`
	// SingleSession: the device allows one session; callers must not
	// run operations on it concurrently
	SingleSession bool `json:"single_session,omitempty"`
}

// Supports reports whether part p is (at least partially) supported.
func (c Capabilities) Supports(p Part) bool { return c.Parts[p].Level != Unsupported }

// Filter returns the parts of ps that are supported. A Capabilities that
// declares no parts at all (a driver relying on Base) keeps every part.
func (c Capabilities) Filter(ps []Part) []Part {
	if len(c.Parts) == 0 {
		return append([]Part(nil), ps...)
	}
	var out []Part
	for _, p := range ps {
		if c.Supports(p) {
			out = append(out, p)
		}
	}
	return out
}

// HasFeature reports whether f is listed.
func (c Capabilities) HasFeature(f Feature) bool {
	for _, x := range c.Features {
		if x == f {
			return true
		}
	}
	return false
}

// Action returns the spec of a named action.
func (c Capabilities) Action(name string) (ActionSpec, bool) { return FindAction(c.Actions, name) }

// Source reports whether a native source is listed.
func (c Capabilities) Source(name string) (SourceSpec, bool) {
	for _, s := range c.Sources {
		if s.Name == name {
			return s, true
		}
	}
	return SourceSpec{}, false
}

// AllSupported returns a Parts map with every part at level Supported,
// for drivers to start from.
func AllSupported() map[Part]PartSupport {
	m := make(map[Part]PartSupport, len(AllParts))
	for _, p := range AllParts {
		m[p] = PartSupport{Level: Supported}
	}
	return m
}

// PartialSupport returns the support of a part that never sets the
// listed fields (Supported when none is listed).
func PartialSupport(missing ...string) PartSupport {
	if len(missing) == 0 {
		return PartSupport{Level: Supported}
	}
	return PartSupport{Level: Partial, Missing: missing}
}
