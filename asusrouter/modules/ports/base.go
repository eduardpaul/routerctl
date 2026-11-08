// Package ports mirrors asusrouter/modules/ports/.
//
// Port models and the readers shared by the port_status and ethernet
// parsers.
package ports

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/usb"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// ARPortCableState is the diagnostic state of a single ethernet cable
// pair.
type ARPortCableState struct {
	Status int `json:"status"`
	Length int `json:"length"`
}

// ARPortsData is the ports and node-level info reported by a single
// device.
//
// Each entry in Ports is a property mapping that always carries
// `ARPortPropertyNativeName`.
type ARPortsData struct {
	Info  map[ARPortsInfo]any      `json:"info"`
	Ports []map[ARPortProperty]any `json:"ports"`
}

// Identity is the part of the device identity the port parsers use
// (Python passes the whole `ARDeviceIdentity`; a narrow interface keeps
// package ports free of the device import cycle).
type Identity interface {
	Mac() *identifiers.MacAddress
}

var capabilities = func() []ARPortCapability {
	var out []ARPortCapability
	for _, c := range ARPortCapabilityMembers {
		if c >= 0 {
			out = append(out, c)
		}
	}
	return out
}()

type capabilityType struct {
	capability ARPortCapability
	portType   ARPortType
}

// The order is important: first matching capability wins
var portCapabilityToType = []capabilityType{
	{ARPortCapabilityWan, ARPortTypeWan},
	{ARPortCapabilityLan, ARPortTypeLan},
	{ARPortCapabilityUsb, ARPortTypeUsb},
	{ARPortCapabilityMoca, ARPortTypeMoca},
}

// ReadPortType reads the port type from capability bits.
func ReadPortType(caps map[ARPortCapability]bool) ARPortType {
	for _, ct := range portCapabilityToType {
		if caps[ct.capability] {
			return ct.portType
		}
	}
	return ARPortTypeUnknown
}

// ReadPortSpeed reads the port link rate for the given port type: an
// ARPortSpeed, or a usb.ARUSBSpeed for USB ports.
func ReadPortSpeed(portType ARPortType, raw int) any {
	if portType == ARPortTypeUsb {
		return usb.ARUSBSpeedFromValue(raw)
	}
	return ARPortSpeedFromValue(raw)
}

// ReadPortCapabilities reads port capabilities from a raw integer.
func ReadPortCapabilities(raw any) map[ARPortCapability]bool {
	value := converters.RawToInt(raw)
	if value == nil || *value < 0 {
		return map[ARPortCapability]bool{}
	}
	bits := map[int]struct{}{}
	for _, b := range converters.IntToBits(*value) {
		bits[b] = struct{}{}
	}
	out := make(map[ARPortCapability]bool, len(capabilities))
	for _, c := range capabilities {
		_, set := bits[int(c)]
		out[c] = set
	}
	return out
}
