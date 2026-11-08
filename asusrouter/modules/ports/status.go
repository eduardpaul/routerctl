package ports

import (
	"sort"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/usb"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Parser for the get_port_status.cgi endpoint.

// Native-name prefixes used by the modern endpoint
var nativePrefixes = []string{"L", "W", "U"}

type fieldConverter struct {
	rawKey    string
	prop      ARPortProperty
	converter func(any) any
}

// Raw field -> (property, converter) for the simple per-port fields
var portFieldConverters = []fieldConverter{
	{"ifname", ARPortPropertyIfname, converters.AsAny(converters.RawToStr)},
	{"ui_display", ARPortPropertyUiDisplay, converters.AsAny(converters.RawToStr)},
	{"seq_no", ARPortPropertySeqNo, converters.AsAny(converters.RawToInt)},
	{"flag", ARPortPropertyFlag, converters.AsAny(converters.RawToBool)},
	{"phy_port_id", ARPortPropertyPhyPortId, converters.AsAny(converters.RawToInt)},
	{"ext_port_id", ARPortPropertyExtPortId, converters.AsAny(converters.RawToInt)},
	{"timeout", ARPortPropertyTimeout, converters.AsAny(converters.RawToInt)},
	{"linkrecover", ARPortPropertyLinkRecover, converters.AsAny(converters.RawToBool)},
}

func isPyDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// NativeName mirrors `status.native_name`: normalize a modern port label,
// or nil if it is unknown.
//
// Modern labels are already in the target form (`L1`, `W0`, `U1`, `AI`);
// this only validates them and warns once on anything else.
func NativeName(label string) *string {
	if label == "AI" {
		return &label
	}
	first := pySlice(label, 0, 1)
	for _, p := range nativePrefixes {
		if first == p && isPyDigits(pySlice(label, 1, -1)) {
			return &label
		}
	}

	WarnUnknownPort(label)
	return nil
}

// ReadRole resolves the port role from its label and capabilities.
//
// baseRole is the capability-derived type, reused here to avoid
// recomputing it. maxRate is an ARPortSpeed or a usb.ARUSBSpeed.
func ReadRole(name string, baseRole ARPortType, caps map[ARPortCapability]bool, maxRate any) ARPortType {
	// The AI board is a dedicated internal port identified by its label
	if name == "AI" {
		return ARPortTypeAi
	}

	// A 10G-capable port advertising SFP+ is an SFP+ port
	// (IntEnum equality: a USB speed of the same value matches too)
	if speedValue(maxRate) == int(ARPortSpeedMbps10000) && caps[ARPortCapabilitySfpp] {
		return ARPortTypeSfpp
	}

	return baseRole
}

func speedValue(v any) int {
	switch s := v.(type) {
	case ARPortSpeed:
		return int(s)
	case usb.ARUSBSpeed:
		return int(s)
	case int:
		return s
	}
	return int(ARPortSpeedUnknown)
}

// ReadCable collapses the per-pair cable diagnostics into a single
// mapping.
func ReadCable(values map[string]any) map[ARPortCablePair]ARPortCableState {
	cable := map[ARPortCablePair]ARPortCableState{}
	for _, pair := range ARPortCablePairMembers {
		if _, ok := values[string(pair)]; !ok {
			continue
		}
		state := ARPortCableState{}
		if v := converters.RawToInt(values[string(pair)]); v != nil {
			state.Status = *v
		}
		if v := converters.RawToInt(values[string(pair)+"_len"]); v != nil {
			state.Length = *v
		}
		cable[pair] = state
	}

	return cable
}

// ReadDevices parses the USB devices connected to a port, if any.
func ReadDevices(values map[string]any) []usb.ARUSBDevice {
	rawDevices, ok := values["devices"].(map[string]any)
	if !ok {
		return []usb.ARUSBDevice{}
	}

	out := []usb.ARUSBDevice{}
	for _, position := range sortedKeys(rawDevices) {
		if raw, ok := rawDevices[position].(map[string]any); ok {
			out = append(out, usb.ARUSBDeviceFromRaw(position, raw))
		}
	}
	return out
}

// ReadNodeInfo parses the node-level info reported alongside the ports.
func ReadNodeInfo(raw map[string]any) map[ARPortsInfo]any {
	info := map[ARPortsInfo]any{}
	for _, member := range ARPortsInfoMembers {
		v, ok := raw[string(member)]
		if !ok {
			continue
		}
		if member == ARPortsInfoCdGoodToGo {
			info[member] = converters.AsAny(converters.RawToBool)(v)
		} else {
			info[member] = converters.AsAny(converters.RawToInt)(v)
		}
	}

	return info
}

func intOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// ReadPort parses a single modern port entry into a property mapping.
func ReadPort(name string, values map[string]any) map[ARPortProperty]any {
	caps := ReadPortCapabilities(values["cap"])
	baseRole := ReadPortType(caps)

	linkRate := ReadPortSpeed(baseRole, intOrZero(converters.RawToInt(values["link_rate"])))
	maxRate := ReadPortSpeed(baseRole, intOrZero(converters.RawToInt(values["max_rate"])))

	present := []ARPortCapability{}
	for _, c := range capabilities {
		if caps[c] {
			present = append(present, c)
		}
	}

	port := map[ARPortProperty]any{
		ARPortPropertyNativeName:   name,
		ARPortPropertyState:        converters.AsAny(converters.RawToBool)(values["is_on"]),
		ARPortPropertyLinkRate:     linkRate,
		ARPortPropertyMaxRate:      maxRate,
		ARPortPropertyRole:         ReadRole(name, baseRole, caps, maxRate),
		ARPortPropertyCapabilities: present,
		ARPortPropertyExtended:     true,
	}

	for _, fc := range portFieldConverters {
		if v, ok := values[fc.rawKey]; ok {
			port[fc.prop] = fc.converter(v)
		}
	}

	if cable := ReadCable(values); len(cable) > 0 {
		port[ARPortPropertyCable] = cable
	}

	if devices := ReadDevices(values); len(devices) > 0 {
		port[ARPortPropertyDevices] = devices
	}

	return port
}

// sortedKeys returns dict keys sorted (Python iterates parsed JSON in
// document order, which Go maps lose; sorting keeps the result stable).
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// portLabelOrder sorts modern labels like the router lists them: WAN
// first, then LAN, then USB, then the AI board, each by index.
func portLabelOrder(labels []string) {
	rank := func(label string) (int, int) {
		r := 4
		switch {
		case label == "AI":
			r = 3
		case strings.HasPrefix(label, "W"):
			r = 0
		case strings.HasPrefix(label, "L"):
			r = 1
		case strings.HasPrefix(label, "U"):
			r = 2
		}
		idx := 0
		if v := converters.RawToInt(pySlice(label, 1, -1)); v != nil {
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

// TranslatePortStatus translates a get_port_status.cgi payload to the
// unified format.
func TranslatePortStatus(data map[string]any, identity Identity) map[identifiers.MacAddress]ARPortsData {
	nodeInfo, _ := data["node_info"].(map[string]any)
	portInfo, _ := data["port_info"].(map[string]any)

	// Preserve order: ports first, then any node-info-only MACs (the
	// result is a dict keyed by MAC, so the order only decides which raw
	// key wins when two spell the same MAC)
	macs := sortedKeys(portInfo)
	for _, m := range sortedKeys(nodeInfo) {
		if _, ok := portInfo[m]; !ok {
			macs = append(macs, m)
		}
	}

	result := map[identifiers.MacAddress]ARPortsData{}
	for _, macRaw := range macs {
		mac := identifiers.MacAddressFromValueSafe(macRaw)
		if mac == nil {
			continue
		}

		out := []map[ARPortProperty]any{}
		labelsMap, _ := portInfo[macRaw].(map[string]any)
		labels := make([]string, 0, len(labelsMap))
		for label := range labelsMap {
			labels = append(labels, label)
		}
		portLabelOrder(labels)
		for _, label := range labels {
			name := NativeName(label)
			if name == nil {
				continue
			}
			values, _ := labelsMap[label].(map[string]any)
			out = append(out, ReadPort(*name, values))
		}

		info, _ := nodeInfo[macRaw].(map[string]any)
		result[*mac] = ARPortsData{Info: ReadNodeInfo(info), Ports: out}
	}

	return result
}
