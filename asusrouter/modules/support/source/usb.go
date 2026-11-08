package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/usb"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// First match wins
var usbGeneration = support.MakeEnumTranslator([]support.TableEntry[usb.ARUSBGeneration]{
	{Key: string(support.ARSupportValueUsb3), Value: usb.ARUSBGenerationUsb3},
	{Key: string(support.ARSupportValueUsb2), Value: usb.ARUSBGenerationUsb2},
	{Key: string(support.ARSupportValueUsb), Value: usb.ARUSBGenerationUsb},
}, usb.ARUSBGenerationUnknown)

// translateUsbCapabilities maps advertised USB capabilities; legacy
// devices omit the port count.
func translateUsbCapabilities(data map[string]any) map[usb.ARUSBCapability]any {
	capabilities := map[usb.ARUSBCapability]any{}

	if generation := usbGeneration(data); generation != usb.ARUSBGenerationUnknown {
		capabilities[usb.ARUSBCapabilityGeneration] = generation
	}

	if readers.IsTrueInDict(string(support.ARSupportValueUsbModem), data) &&
		!readers.IsTrueInDict(string(support.ARSupportValueUsbNoModem), data) {
		capabilities[usb.ARUSBCapabilityModem] = true
	}

	if ports := converters.RawToInt(data[string(support.ARSupportValueUsbPorts)]); ports != nil && *ports != 0 {
		capabilities[usb.ARUSBCapabilityPortsCount] = *ports
	}

	if readers.IsTrueInDict(string(support.ARSupportValueUsbWan), data) {
		capabilities[usb.ARUSBCapabilityWan] = true
	}
	return capabilities
}

// translateUsb reports whether the device advertises any USB capability.
func translateUsb(data map[string]any) bool {
	return len(translateUsbCapabilities(data)) > 0
}
