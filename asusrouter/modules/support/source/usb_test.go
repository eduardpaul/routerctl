package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/usb"
)

func TestTranslateUsb(t *testing.T) {
	check(t, "translateUsb", translateUsb, []boolCase{
		{d{k(support.ARSupportValueUsb): 1}, true},
		{d{k(support.ARSupportValueUsbModem): 1}, true},
		{d{}, false},
	})
}

func TestTranslateUsbCapabilities(t *testing.T) {
	type caps = map[usb.ARUSBCapability]any
	check(t, "translateUsbCapabilities", translateUsbCapabilities, []struct {
		data     d
		expected caps
	}{
		{d{}, caps{}},
		// Generation, highest wins
		{d{k(support.ARSupportValueUsb3): 1, k(support.ARSupportValueUsb2): 1}, caps{usb.ARUSBCapabilityGeneration: usb.ARUSBGenerationUsb3}},
		// Legacy: USB present but no port count reported
		{d{k(support.ARSupportValueUsb2): 1}, caps{usb.ARUSBCapabilityGeneration: usb.ARUSBGenerationUsb2}},
		// Port count included only when reported
		{
			d{k(support.ARSupportValueUsb): 1, k(support.ARSupportValueUsbPorts): 2},
			caps{usb.ARUSBCapabilityGeneration: usb.ARUSBGenerationUsb, usb.ARUSBCapabilityPortsCount: 2},
		},
		// Modem and WAN as bool capabilities
		{
			d{k(support.ARSupportValueUsb3): 1, k(support.ARSupportValueUsbModem): 1, k(support.ARSupportValueUsbWan): 1},
			caps{
				usb.ARUSBCapabilityGeneration: usb.ARUSBGenerationUsb3,
				usb.ARUSBCapabilityModem:      true,
				usb.ARUSBCapabilityWan:        true,
			},
		},
		// nomodem disables the modem capability even when modem is set
		{
			d{k(support.ARSupportValueUsb): 1, k(support.ARSupportValueUsbModem): 1, k(support.ARSupportValueUsbNoModem): 1},
			caps{usb.ARUSBCapabilityGeneration: usb.ARUSBGenerationUsb},
		},
		// A zero port count is dropped
		{
			d{k(support.ARSupportValueUsb): 1, k(support.ARSupportValueUsbPorts): "0"},
			caps{usb.ARUSBCapabilityGeneration: usb.ARUSBGenerationUsb},
		},
	})
}
