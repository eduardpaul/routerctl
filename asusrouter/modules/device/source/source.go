// Package source mirrors asusrouter/modules/device/source.py (split out
// of package device: it registers with the registry, which imports the
// identity).
package source

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"

	// The device request needs the nvram and support sources registered
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/support/source"
)

// ARDeviceSource is the AsusRouter device.
type ARDeviceSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARDeviceSource) SourceName() string { return "ARDeviceSource" }

// ARDeviceSourceUniversal is a universal instance of the device source
// (preferred).
var ARDeviceSourceUniversal = &ARDeviceSource{}

// DeviceRequest mirrors `DEVICE_REQUEST`.
var DeviceRequest = func() []source.DataSource {
	out := []source.DataSource{
		// Device information
		nvram.ARNvramTypeMac,
		nvram.ARNvramTypeMacLan,
		nvram.ARNvramTypeMacWan,
		nvram.ARNvramTypeModel,
		nvram.ARNvramTypeModelOriginal,
		nvram.ARNvramTypeSecretCode,
		nvram.ARNvramTypeSerial,
		nvram.ARNvramTypeWirelessBands,
	}
	// Per-radio band type, for the wifi fallback when WIRELESS_BANDS is empty
	for i := 0; i < wifi.ARWifiMaxUnits; i++ {
		out = append(out, nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, i))
	}
	return append(out,
		// Firmware information
		nvram.ARNvramTypeFwMajor,
		nvram.ARNvramTypeFwMinor,
		nvram.ARNvramTypeFwBuild,
		nvram.ARNvramTypeFwSwpj,
		// Software information
		nvram.ARNvramTypeReMode,
		nvram.ARNvramTypeSwMode,
		nvram.ARNvramTypeWlcProxySta,
		support.ARSupportSourceUniversal,
	)
}()

// fetchState fetches the device state.
//
// This is a special case where we need to fetch data from other sources.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	if c.FetchDataMulti == nil {
		return device.IdentityData{}, nil
	}
	data, err := c.FetchDataMulti(ctx, DeviceRequest, nil)
	if err != nil {
		return nil, err
	}
	return device.IdentityData(data), nil
}

// translateState translates the device state.
func translateState(raw any, tc *registry.TranslateContext) any {
	switch data := raw.(type) {
	case device.IdentityData:
		return device.ARDeviceIdentityBuild(data)
	case map[source.ID]any:
		return device.ARDeviceIdentityBuild(device.IdentityData(data))
	}
	return device.ARDeviceIdentityBuild(device.IdentityData{})
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARDeviceSource",
		Default:   func() source.DataSource { return &ARDeviceSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
