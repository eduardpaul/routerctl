package aimesh

import (
	"fmt"
	"log/slog"
	"math/bits"
	"sort"
	"strconv"
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// capability index -> unknown bit positions already reported, so each
// distinct unknown bit is warned about only once
var (
	reportedUnknownBits = map[int]map[int]struct{}{}
	unknownCapsLock     sync.Mutex
)

// Unmapped bits we already know about but cannot decode yet. We keep the
// raw value, but do not warn about these - they are not new data
var acknowledgedUnknown = map[int]map[int]struct{}{
	4: {6: {}, 11: {}, 13: {}, 15: {}, 19: {}, 23: {}, 24: {}, 25: {}},
}

type capBit struct {
	bit     int
	feature ARAiMeshFeature
}

type capIndex struct {
	index int
	bits  []capBit
}

// Capability index -> {bit -> feature} (in Python dict order)
var capabilityMap = []capIndex{
	{1, []capBit{
		{0, ARAiMeshFeatureCentralLed},
		{1, ARAiMeshFeatureLp55xxLed},
		{2, ARAiMeshFeatureLedOnOff},
		{3, ARAiMeshFeatureLedBrightness},
		{4, ARAiMeshFeatureLedAura},
		{5, ARAiMeshFeatureLedNightMode},
		{6, ARAiMeshFeatureCentralLedOnOff},
	}},
	{2, []capBit{{0, ARAiMeshFeatureManualReboot}}},
	{3, []capBit{{0, ARAiMeshFeaturePreferableBackhaul}, {1, ARAiMeshFeaturePreferNodeApply}}},
	{4, []capBit{
		{0, ARAiMeshFeatureUsb},
		{1, ARAiMeshFeatureGuestNetwork},
		{2, ARAiMeshFeatureWpa3},
		{3, ARAiMeshFeatureVifOnboarding},
		{4, ARAiMeshFeatureSchedV2},
		{5, ARAiMeshFeatureWifiRadio},
		{8, ARAiMeshFeatureSwitchctrl},
		{9, ARAiMeshFeaturePortStatus},
		{10, ARAiMeshFeatureLocalAccess},
		{16, ARAiMeshFeatureWpa3Enterprise},
		{20, ARAiMeshFeatureMloBh},
		{21, ARAiMeshFeatureMloFh},
		{22, ARAiMeshFeatureSmartHomeMasterUi},
	}},
	{5, []capBit{{0, ARAiMeshFeatureLacp}}},
	{6, []capBit{{0, ARAiMeshFeatureGn2g1}, {1, ARAiMeshFeatureGn2g2}, {2, ARAiMeshFeatureGn2g3}}},
	{7, []capBit{{0, ARAiMeshFeatureGn5g1}, {1, ARAiMeshFeatureGn5g2}, {2, ARAiMeshFeatureGn5g3}}},
	{8, []capBit{{0, ARAiMeshFeatureGn5gh1}, {1, ARAiMeshFeatureGn5gh2}, {2, ARAiMeshFeatureGn5gh3}}},
	{15, []capBit{{0, ARAiMeshFeatureWansCapWan}}},
	{16, []capBit{{0, ARAiMeshFeatureManualReconn}}},
	{17, []capBit{{0, ARAiMeshFeatureManualForceRoaming}}},
	{18, []capBit{
		{0, ARAiMeshFeatureFronthaulApOptionOff},
		{1, ARAiMeshFeatureFronthaulApOptionAuto},
		{2, ARAiMeshFeatureFronthaulApOptionOn},
	}},
	{19, []capBit{{0, ARAiMeshFeatureManualStaBinding}}},
	{20, []capBit{{0, ARAiMeshFeatureManualResetDefault}}},
	{22, []capBit{
		{0, ARAiMeshFeatureWifiRadio0},
		{1, ARAiMeshFeatureWifiRadio1},
		{2, ARAiMeshFeatureWifiRadio2},
		{3, ARAiMeshFeatureWifiRadio3},
		{4, ARAiMeshFeatureWifiRadio4},
	}},
	{23, []capBit{{0, ARAiMeshFeatureEthernetBackhaulMode}}},
	{24, []capBit{{0, ARAiMeshFeatureGn6g1}, {1, ARAiMeshFeatureGn6g2}, {2, ARAiMeshFeatureGn6g3}}},
	{34, []capBit{{0, ARAiMeshFeatureGn6gh1}, {1, ARAiMeshFeatureGn6gh2}, {2, ARAiMeshFeatureGn6gh3}}},
}

func capabilityBits(index int) ([]capBit, bool) {
	for _, entry := range capabilityMap {
		if entry.index == index {
			return entry.bits, true
		}
	}
	return nil, false
}

// warnUnknownBits warns once per distinct unmapped bit of a known
// capability index.
func warnUnknownBits(index int, value int, knownMask int) {
	unknown := value &^ knownMask
	if unknown == 0 {
		return
	}

	// Python ints are unbounded; a negative value has infinitely many
	// set high bits, which Go cannot list - use the 64-bit pattern
	u := uint64(unknown)
	acknowledged := acknowledgedUnknown[index]
	unknownCapsLock.Lock()
	defer unknownCapsLock.Unlock()
	seen, ok := reportedUnknownBits[index]
	if !ok {
		seen = map[int]struct{}{}
		reportedUnknownBits[index] = seen
	}
	var newBits []int
	for bit := 0; bit < 64-bits.LeadingZeros64(u); bit++ {
		if u&(1<<uint(bit)) == 0 {
			continue
		}
		if _, s := seen[bit]; s {
			continue
		}
		if _, a := acknowledged[bit]; a {
			continue
		}
		newBits = append(newBits, bit)
	}
	if len(newBits) == 0 {
		return
	}
	sort.Ints(newBits)
	for _, b := range newBits {
		seen[b] = struct{}{}
	}
	slog.Warn(fmt.Sprintf(
		"We found unknown bits `%s` in AiMesh capability `%d`. "+
			"Please, report this raw value: `%d`",
		converters.PyRepr(newBits), index, value,
	))
}

// FeatureSet is a Python `frozenset[ARAiMeshFeature]`; it canonicalizes as
// a sorted list.
type FeatureSet map[ARAiMeshFeature]struct{}

// Has reports whether the feature is in the set.
func (s FeatureSet) Has(f ARAiMeshFeature) bool {
	_, ok := s[f]
	return ok
}

// Sorted returns the features sorted by value.
func (s FeatureSet) Sorted() []ARAiMeshFeature {
	out := make([]ARAiMeshFeature, 0, len(s))
	for f := range s {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// CanonicalValue renders the set as a sorted list.
func (s FeatureSet) CanonicalValue() any {
	out := make([]any, 0, len(s))
	for _, f := range s.Sorted() {
		out = append(out, string(f))
	}
	return out
}

// TranslateFeatures decodes the supported AiMesh features from a
// capability dict.
func TranslateFeatures(capability any) FeatureSet {
	capMap, ok := capability.(map[string]any)
	if !ok {
		return FeatureSet{}
	}

	features := FeatureSet{}
	for _, entry := range capabilityMap {
		value := converters.RawToInt(capMap[strconv.Itoa(entry.index)])
		if value == nil {
			continue
		}
		knownMask := 0
		for _, b := range entry.bits {
			knownMask |= 1 << uint(b.bit)
			if *value&(1<<uint(b.bit)) != 0 {
				features[b.feature] = struct{}{}
			}
		}
		warnUnknownBits(entry.index, *value, knownMask)
	}

	return features
}

// IsFullyDecoded reports whether every set bit of a capability value is
// mapped.
//
// Such a value is fully represented by the decoded features and can be
// dropped from the raw passthrough; one with unmapped bits is kept.
func IsFullyDecoded(key string, value any) bool {
	index := converters.RawToInt(key)
	if index == nil {
		return false
	}
	capBits, ok := capabilityBits(*index)
	if !ok {
		return false
	}

	number := converters.RawToInt(value)
	if number == nil {
		return false
	}

	known := 0
	for _, b := range capBits {
		known |= 1 << uint(b.bit)
	}
	return *number&^known == 0
}
