package ports

import (
	"sort"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Parser for the ajax_ethernet_ports.asp endpoint.
//
// The legacy endpoint only reports the current link rate per port and has
// no MAC, so the ports are attributed to the main device via its identity.

// Legacy label prefix -> native-name letter
var legacyPrefixes = map[string]string{
	"WAN": "W",
	"LAN": "L",
	"USB": "U",
}

var downSpeeds = []ARPortSpeed{ARPortSpeedDown, ARPortSpeedUnknown}

// Single-letter speed codes used by the ajax_ethernet_ports endpoint
var portSpeedMap = map[string]ARPortSpeed{
	"t": ARPortSpeedMbps10,
	"X": ARPortSpeedDown,
	"M": ARPortSpeedMbps100,
	"G": ARPortSpeedMbps1000,
	"Q": ARPortSpeedMbps2500,
	"F": ARPortSpeedMbps5000,
	"T": ARPortSpeedMbps10000,
}

// ReadEthernetPortSpeed reads the port speed from the legacy single-letter
// router code.
func ReadEthernetPortSpeed(code string) ARPortSpeed {
	if speed, ok := portSpeedMap[code]; ok {
		return speed
	}
	return ARPortSpeedUnknown
}

// pySlice mirrors Python `s[start:end]` on code points (end < 0 means to
// the end).
func pySlice(s string, start, end int) string {
	r := []rune(s)
	if start > len(r) {
		start = len(r)
	}
	if end < 0 || end > len(r) {
		end = len(r)
	}
	return string(r[start:end])
}

// LegacyNativeName mirrors `legacy.native_name`: normalize a legacy port
// label to the modern native name.
//
// `LAN 1` -> `L1`, `WAN 0` -> `W0`, a 10G port -> `SFP`. Unknown labels
// are skipped with a one-time warning.
func LegacyNativeName(label string) *string {
	prefix := strings.ToUpper(pySlice(label, 0, 3))

	if prefix == "10G" {
		sfp := "SFP"
		return &sfp
	}

	letter, ok := legacyPrefixes[prefix]
	index := converters.RawToInt(pySlice(label, 3, -1))
	if !ok || index == nil {
		WarnUnknownPort(label)
		return nil
	}

	name := letter + converters.PyStr(*index)
	return &name
}

// legacyLabelOrder sorts legacy labels like the router lists them (WAN,
// then LAN, then USB, by index); Python keeps the JSON document order,
// which Go maps lose.
func legacyLabelOrder(labels []string) {
	rank := func(label string) (int, int) {
		r := 3
		switch strings.ToUpper(pySlice(label, 0, 3)) {
		case "WAN":
			r = 0
		case "LAN":
			r = 1
		case "10G":
			r = 2
		case "USB":
			r = 4
		}
		idx := 0
		if v := converters.RawToInt(pySlice(label, 3, -1)); v != nil {
			idx = *v
		}
		return r, idx
	}
	sort.SliceStable(labels, func(i, j int) bool {
		ri, ii := rank(labels[i])
		rj, ij := rank(labels[j])
		if ri != rj {
			return ri < rj
		}
		if ii != ij {
			return ii < ij
		}
		return labels[i] < labels[j]
	})
}

// TranslateEthernetPorts translates an ajax_ethernet_ports.asp payload to
// the unified format.
func TranslateEthernetPorts(data map[string]any, identity Identity) map[identifiers.MacAddress]ARPortsData {
	portSpeed, _ := data["portSpeed"].(map[string]any)
	if len(portSpeed) == 0 {
		return map[identifiers.MacAddress]ARPortsData{}
	}

	var mac *identifiers.MacAddress
	if identity != nil {
		mac = identity.Mac()
	}
	if mac == nil {
		return map[identifiers.MacAddress]ARPortsData{}
	}

	labels := make([]string, 0, len(portSpeed))
	for label := range portSpeed {
		labels = append(labels, label)
	}
	legacyLabelOrder(labels)

	out := []map[ARPortProperty]any{}
	for _, label := range labels {
		name := LegacyNativeName(label)
		if name == nil {
			continue
		}
		code, _ := portSpeed[label].(string)
		linkRate := ReadEthernetPortSpeed(code)
		state := true
		for _, d := range downSpeeds {
			if linkRate == d {
				state = false
			}
		}
		out = append(out, map[ARPortProperty]any{
			ARPortPropertyNativeName: *name,
			ARPortPropertyState:      state,
			ARPortPropertyLinkRate:   linkRate,
			ARPortPropertyExtended:   false,
		})
	}

	return map[identifiers.MacAddress]ARPortsData{
		*mac: {Info: map[ARPortsInfo]any{}, Ports: out},
	}
}
