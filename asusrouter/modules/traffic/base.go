// Package traffic mirrors asusrouter/modules/traffic/.
//
// Python `traffic/source.py` imports the `aimesh` and `interface`
// subpackages, which import this package's base; Go forbids that cycle, so
// the `ARTrafficSource` fetcher lives in the `traffic/source` subpackage.
package traffic

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// ARTrafficLink is a traffic link: a fixed type (ARTrafficType) or a
// specific wifi band (wifi.ARWiFiBand, wireless). nil means all links.
type ARTrafficLink = any

// CoerceLink mirrors `_coerce_link`: a traffic link (band preferred), or
// nil.
func CoerceLink(value any) ARTrafficLink {
	switch v := value.(type) {
	case nil:
		return nil
	case ARTrafficType:
		return v
	case wifi.ARWiFiBand:
		return v
	}
	if band := wifi.ARWiFiBandFromValue(value); band != wifi.ARWiFiBandUnknown {
		return band
	}
	if trafficType := ARTrafficTypeFromValue(value); trafficType != ARTrafficTypeUnknown {
		return trafficType
	}
	return nil
}

// ARTrafficSource mirrors `ARTrafficSource`: the traffic data source for
// one device (`target`) and `link`.
//
// `target` nil means the connected router; `link` nil means all of the
// device's available links.
type ARTrafficSource struct {
	target *identifiers.MacAddress
	link   ARTrafficLink
}

// NewARTrafficSource initializes the traffic source.
func NewARTrafficSource(link, target any) *ARTrafficSource {
	s := &ARTrafficSource{}
	s.Init(link, target)
	return s
}

// Init sets the defining fields (the Python `__init__`), for embedders.
func (s *ARTrafficSource) Init(link, target any) {
	s.SetTarget(target)
	s.SetLink(link)
}

// SourceName returns the Python class name.
func (*ARTrafficSource) SourceName() string { return "ARTrafficSource" }

// Target gets the target device MAC address.
func (s *ARTrafficSource) Target() *identifiers.MacAddress { return s.target }

// SetTarget sets the target device MAC address.
func (s *ARTrafficSource) SetTarget(value any) {
	s.target = identifiers.MacAddressFromValueSafe(value)
}

// Link gets the traffic link (type or band), or nil for all.
func (s *ARTrafficSource) Link() ARTrafficLink { return s.link }

// SetLink sets the traffic link.
func (s *ARTrafficSource) SetLink(value any) {
	s.link = CoerceLink(value)
}

// SourceKey keys by the target device and link.
func (s *ARTrafficSource) SourceKey() []any {
	var target any
	if s.target != nil {
		target = *s.target
	}
	return []any{target, s.link}
}
