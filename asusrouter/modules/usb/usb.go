// Package usb mirrors asusrouter/modules/usb/__init__.py.
//
// The enums (ARUSBCapability, ARUSBGeneration, ARUSBSpeed,
// ARUSBDeviceType) are generated in usb_enums_gen.go.
package usb

import (
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/units"
)

// Asus reports sizes in KiB despite labelling them `_kb`
var kibToByte = func() func(float64) float64 {
	f, err := units.DataUnitConverter.ConverterFactory(units.UnitOfDataKibibyte, units.UnitOfDataByte)
	if err != nil {
		panic(err)
	}
	return f
}()

// Sentinel the device uses for unavailable string fields
const unavailable = "-1"

// cleanStr reads a string field, mapping the `-1` sentinel to nil.
func cleanStr(value any) *string {
	text := converters.RawToStr(value)
	if text == nil || *text == unavailable {
		return nil
	}
	return text
}

// kibToBytes converts a KiB value to bytes, or nil when unavailable.
func kibToBytes(value any) *int {
	kib := converters.RawToInt(value)
	if kib == nil {
		return nil
	}
	b := int(kibToByte(float64(*kib)))
	return &b
}

// ARUSBDevice mirrors `ARUSBDevice`: a device connected to a USB port.
//
// Identity is the raw serial and manufacturer; everything else
// (location, capacity, negotiated speed, usage) is descriptive and
// excluded from equality so the same device matches across fetches
// and reconnects. Use Equal / IdentityKey for the Python `__eq__` /
// `__hash__`.
type ARUSBDevice struct {
	// Identity
	Serial          *string `json:"serial"`
	ManufacturerRaw *string `json:"manufacturer_raw"`

	// Descriptive — not part of identity
	Type     ARUSBDeviceType `json:"type"`
	Product  *string         `json:"product"`
	Name     *string         `json:"name"`
	Speed    ARUSBSpeed      `json:"speed"`
	Position *string         `json:"position"`
	Size     *int            `json:"size"`
	Used     *int            `json:"used"`
}

// NewARUSBDevice returns a device with the Python dataclass defaults.
func NewARUSBDevice() ARUSBDevice {
	return ARUSBDevice{Type: ARUSBDeviceTypeUnknown, Speed: ARUSBSpeedUnknown}
}

// ARUSBDeviceFromRaw mirrors `ARUSBDevice.from_raw`: build a device from a
// raw `devices` entry.
func ARUSBDeviceFromRaw(position string, raw map[string]any) ARUSBDevice {
	speed := 0
	if v := converters.RawToInt(raw["speed"]); v != nil && *v != 0 {
		speed = *v
	}
	return ARUSBDevice{
		Serial:          cleanStr(raw["serial"]),
		ManufacturerRaw: cleanStr(raw["manu"]),
		Type:            ARUSBDeviceTypeFromValue(raw["type"]),
		Product:         cleanStr(raw["product"]),
		Name:            cleanStr(raw["dev_name"]),
		Speed:           ARUSBSpeedFromValue(speed),
		Position:        &position,
		Size:            kibToBytes(raw["size_kb"]),
		Used:            kibToBytes(raw["used_kb"]),
	}
}

// ARUSBDeviceKey is the comparable identity of a device (serial and raw
// manufacturer; nil is distinct from any string).
type ARUSBDeviceKey struct {
	HasSerial       bool
	Serial          string
	HasManufacturer bool
	ManufacturerRaw string
}

// IdentityKey mirrors `__hash__`: the comparable identity, usable as a map
// key to group devices like a Python set.
func (d ARUSBDevice) IdentityKey() ARUSBDeviceKey {
	var k ARUSBDeviceKey
	if d.Serial != nil {
		k.HasSerial, k.Serial = true, *d.Serial
	}
	if d.ManufacturerRaw != nil {
		k.HasManufacturer, k.ManufacturerRaw = true, *d.ManufacturerRaw
	}
	return k
}

// Equal mirrors `__eq__`: equal by serial and raw manufacturer only.
func (d ARUSBDevice) Equal(other ARUSBDevice) bool {
	return d.IdentityKey() == other.IdentityKey()
}
