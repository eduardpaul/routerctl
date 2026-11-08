// Package device mirrors asusrouter/modules/device/ (the identity and its
// recovery); the data source lives in `device/source`.
package device

import (
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// IdentityData is the raw device payload (Python `Mapping[Any, Any]`):
// the content of every `DEVICE_REQUEST` source, keyed by source ID.
type IdentityData map[source.ID]any

// get returns the content of a source.
func (d IdentityData) get(src source.DataSource) any {
	return d[source.IDOf(src)]
}

// translateFirmware builds firmware information from raw data.
func translateFirmware(data IdentityData) *firmware.ARFirmware {
	return firmware.ARFirmwareFromNvram(
		data.get(nvram.ARNvramTypeFwMajor),
		data.get(nvram.ARNvramTypeFwMinor),
		data.get(nvram.ARNvramTypeFwBuild),
	)
}

// wifiUnits reads the `UNITS` WiFi capability ([]int).
func wifiUnits(capabilities any) []int {
	caps, ok := capabilities.(map[wifi.ARWiFiCapability]any)
	if !ok {
		return nil
	}
	switch v := caps[wifi.ARWiFiCapabilityUnits].(type) {
	case []int:
		return v
	case []any:
		out := make([]int, 0, len(v))
		for _, item := range v {
			if n := converters.RawToInt(item); n != nil {
				out = append(out, *n)
			}
		}
		return out
	}
	return nil
}

// bandMap is a band -> unit dict that keeps the Python insertion order.
type bandMap struct {
	units map[wifi.ARWiFiBand]int
	order []wifi.ARWiFiBand
}

func newBandMap() bandMap { return bandMap{units: map[wifi.ARWiFiBand]int{}} }

func (b *bandMap) set(band wifi.ARWiFiBand, unit int) {
	if _, ok := b.units[band]; !ok {
		b.order = append(b.order, band)
	}
	b.units[band] = unit
}

// wifiFromBands maps bands via `WIRELESS_BANDS` nvram and the WiFi units
// support.
func wifiFromBands(data IdentityData, sup map[support.ARSupportType]any) bandMap {
	bands := readers.SplitRows(data.get(nvram.ARNvramTypeWirelessBands))
	bandsIDs := wifiUnits(sup[support.ARSupportTypeWifiCapabilities])

	result := newBandMap()
	for i := 0; i < len(bands) && i < len(bandsIDs); i++ {
		// `ARWiFiBand(band)` - a strict value lookup
		bandEnum := wifi.ARWiFiBand(bands[i])
		if !bandEnum.Valid() || bandEnum == wifi.ARWiFiBandUnknown {
			continue
		}
		result.set(bandEnum, bandsIDs[i])
	}

	return result
}

// wifiFromNband derives bands from per-radio `wl{}_nband` (older firmware
// fallback).
func wifiFromNband(data IdentityData) bandMap {
	result := newBandMap()
	for unit := 0; unit < wifi.ARWifiMaxUnits; unit++ {
		band := wifi.ARWiFiBandFromNband(
			converters.RawToStr(data.get(nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, unit))),
		)
		if _, seen := result.units[band]; band != wifi.ARWiFiBandUnknown && !seen {
			result.set(band, unit)
		}
	}

	return result
}

// translateWifi parses the WiFi band -> unit map from the raw device
// payload.
func translateWifi(data IdentityData, sup map[support.ARSupportType]any) bandMap {
	if result := wifiFromBands(data, sup); len(result.units) > 0 {
		return result
	}
	return wifiFromNband(data)
}

// translateMac picks the device MAC, falling back when the label MAC is
// blank.
func translateMac(data IdentityData) *identifiers.MacAddress {
	for _, key := range []nvram.ARNvramType{nvram.ARNvramTypeMac, nvram.ARNvramTypeMacLan, nvram.ARNvramTypeMacWan} {
		if mac := identifiers.MacAddressFromValueSafe(data.get(key)); mac != nil {
			return mac
		}
	}
	return nil
}

// Proxy-STA (`wlc_psta`) states that refine an AP/repeater `sw_mode`
const (
	pstaBridge     = 1 // media bridge over a repeater or AP base
	pstaRepeater   = 2 // repeater over an AP base
	pstaBridgeOnAp = 3 // media bridge over an AP base
)

const reModeAimeshNode = 1 // `re_mode` value marking an AiMesh node

func eqInt(v *int, want int) bool { return v != nil && *v == want }

// translateOperationMode resolves the active operation mode from
// `sw_mode` and `wlc_psta`.
func translateOperationMode(data IdentityData) common.AROperationMode {
	if eqInt(converters.RawToInt(data.get(nvram.ARNvramTypeReMode)), reModeAimeshNode) {
		return common.AROperationModeAimeshNode
	}

	var swMode any
	if v := converters.RawToInt(data.get(nvram.ARNvramTypeSwMode)); v != nil {
		swMode = *v
	}
	mode := common.AROperationModeFromValue(swMode)
	psta := converters.RawToInt(data.get(nvram.ARNvramTypeWlcProxySta))

	repeaterOrAp := mode == common.AROperationModeRepeater || mode == common.AROperationModeAccessPoint
	if (repeaterOrAp && eqInt(psta, pstaBridge)) ||
		(mode == common.AROperationModeAccessPoint && eqInt(psta, pstaBridgeOnAp)) {
		return common.AROperationModeMediaBridge
	}
	if mode == common.AROperationModeAccessPoint && eqInt(psta, pstaRepeater) {
		return common.AROperationModeRepeater
	}
	return mode
}

// translateIdentityBase parses the base identity information from the
// raw payload: (mac, model, model_original, serial).
func translateIdentityBase(data IdentityData) (*identifiers.MacAddress, *string, *string, *string) {
	return translateMac(data),
		converters.RawToStr(data.get(nvram.ARNvramTypeModel)),
		converters.RawToStr(data.get(nvram.ARNvramTypeModelOriginal)),
		converters.RawToStr(data.get(nvram.ARNvramTypeSerial))
}

// ARDeviceIdentity is the AsusRouter device identity. It is safe for
// concurrent use.
type ARDeviceIdentity struct {
	mu sync.RWMutex

	brand         string
	firmware      *firmware.ARFirmware
	mac           *identifiers.MacAddress
	model         *string
	modelOriginal *string
	operationMode common.AROperationMode
	serial        *string
	support       map[support.ARSupportType]any
	// Login name used to reach the device - injected
	username *identifiers.Username
	wifi     bandMap
	// Live AiMesh topology - the only mutable identity part, swapped
	// atomically as a whole snapshot by `UpdateAimesh`
	aimesh *aimesh.ARAiMeshTopology
	// Live boot time - the stabilization anchor; seeded or fetched and
	// kept in sync by `UpdateBoottime`
	boottime *time.Time
	// Seconds the device has been running
	uptime *int
	// The device's own clock as of the last read
	deviceTime *time.Time
	// Edge flag - set when the uptime falls back (a reboot)
	rebooted bool
}

// NewARDeviceIdentity initializes the device identity.
func NewARDeviceIdentity() *ARDeviceIdentity {
	return &ARDeviceIdentity{
		brand:         consts.DefaultIdentityBrand,
		firmware:      firmware.NewARFirmware(nil, nil, nil, nil, false),
		operationMode: common.AROperationModeUnknown,
		support:       map[support.ARSupportType]any{},
		wifi:          newBandMap(),
		aimesh:        aimesh.NewARAiMeshTopology(),
	}
}

// Brand gets the brand.
func (i *ARDeviceIdentity) Brand() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.brand
}

// Firmware gets the firmware information (never nil).
func (i *ARDeviceIdentity) Firmware() *firmware.ARFirmware {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.firmware == nil {
		return firmware.NewARFirmware(nil, nil, nil, nil, false)
	}
	return i.firmware
}

// Mac gets the MAC address.
func (i *ARDeviceIdentity) Mac() *identifiers.MacAddress {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.mac
}

// Model gets the model.
func (i *ARDeviceIdentity) Model() *string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.model
}

// ModelOriginal gets the original model.
func (i *ARDeviceIdentity) ModelOriginal() *string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.modelOriginal
}

// OperationMode gets the active operation mode.
func (i *ARDeviceIdentity) OperationMode() common.AROperationMode {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.operationMode
}

// Serial gets the serial number.
func (i *ARDeviceIdentity) Serial() *string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.serial
}

// Support gets the support information. The map is shared; treat it as
// read-only.
func (i *ARDeviceIdentity) Support() map[support.ARSupportType]any {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.support == nil {
		return map[support.ARSupportType]any{}
	}
	return i.support
}

// Wifi gets the WiFi information (band -> wireless unit). The map is a
// copy.
func (i *ARDeviceIdentity) Wifi() map[wifi.ARWiFiBand]int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make(map[wifi.ARWiFiBand]int, len(i.wifi.units))
	for band, unit := range i.wifi.units {
		out[band] = unit
	}
	return out
}

// WifiBands lists the bands of Wifi() in the Python dict order (the
// order the device reports them in). Go addition: maps do not keep it.
func (i *ARDeviceIdentity) WifiBands() []wifi.ARWiFiBand {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]wifi.ARWiFiBand, 0, len(i.wifi.order))
	for _, band := range i.wifi.order {
		if _, ok := i.wifi.units[band]; ok {
			out = append(out, band)
		}
	}
	return out
}

// WifiByUnit maps a wireless unit index to its band.
func (i *ARDeviceIdentity) WifiByUnit() map[int]wifi.ARWiFiBand {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make(map[int]wifi.ARWiFiBand, len(i.wifi.units))
	// In insertion order: a later band with the same unit wins
	for _, band := range i.wifi.order {
		if unit, ok := i.wifi.units[band]; ok {
			out[unit] = band
		}
	}
	return out
}

// Aimesh gets the live AiMesh topology (never nil).
func (i *ARDeviceIdentity) Aimesh() *aimesh.ARAiMeshTopology {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.aimesh == nil {
		return aimesh.NewARAiMeshTopology()
	}
	return i.aimesh
}

// Username gets the login name used to reach the device.
func (i *ARDeviceIdentity) Username() *identifiers.Username {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.username
}

// UpdateUsername sets the login name used to reach the device.
func (i *ARDeviceIdentity) UpdateUsername(username *identifiers.Username) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.username = username
}

// UpdateAimesh replaces the AiMesh topology snapshot atomically.
func (i *ARDeviceIdentity) UpdateAimesh(topology *aimesh.ARAiMeshTopology) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.aimesh = topology
}

// Boottime gets the live boot time.
func (i *ARDeviceIdentity) Boottime() *time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.boottime
}

// UpdateBoottime replaces the boot time.
func (i *ARDeviceIdentity) UpdateBoottime(boottime *time.Time) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.boottime = boottime
}

// Uptime gets the seconds the device has been running.
func (i *ARDeviceIdentity) Uptime() *int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.uptime
}

// UpdateUptime replaces the uptime, flagging a reboot when it falls back.
func (i *ARDeviceIdentity) UpdateUptime(uptime *int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	previous := i.uptime
	i.uptime = uptime
	if previous != nil && uptime != nil && *uptime < *previous {
		i.rebooted = true
	}
}

// DeviceTime gets the device's own clock as of the last read.
func (i *ARDeviceIdentity) DeviceTime() *time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.deviceTime
}

// UpdateDeviceTime replaces the device's own clock.
func (i *ARDeviceIdentity) UpdateDeviceTime(deviceTime *time.Time) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.deviceTime = deviceTime
}

// Rebooted reports whether a reboot was detected since it was last
// cleared.
func (i *ARDeviceIdentity) Rebooted() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.rebooted
}

// MarkReboot flags a reboot triggered by an action, before the boot time
// moves.
func (i *ARDeviceIdentity) MarkReboot() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.rebooted = true
}

// ClearRebooted clears the reboot flag after it has been acted upon.
func (i *ARDeviceIdentity) ClearRebooted() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.rebooted = false
}

// ARDeviceIdentityBuild mirrors `ARDeviceIdentity.build`: build the
// device identity from the data.
func ARDeviceIdentityBuild(data IdentityData) *ARDeviceIdentity {
	identity := NewARDeviceIdentity()
	if sup, ok := data.get(support.ARSupportSourceUniversal).(map[support.ARSupportType]any); ok {
		// A shallow copy: recovery adds to it, and the support source
		// state may be read concurrently
		identity.support = make(map[support.ARSupportType]any, len(sup))
		for k, v := range sup {
			identity.support[k] = v
		}
	}
	identity.firmware = translateFirmware(data)
	identity.mac, identity.model, identity.modelOriginal, identity.serial = translateIdentityBase(data)
	identity.operationMode = translateOperationMode(data)
	identity.wifi = translateWifi(data, identity.support)

	// Try to recover missing values indirectly
	recoverSupport(identity)

	return identity
}

// CanonicalValue renders the public properties, like the Python
// canonical form of a plain object.
func (i *ARDeviceIdentity) CanonicalValue() any {
	wifiOut := i.Wifi()
	return map[string]any{
		"aimesh":         i.Aimesh(),
		"boottime":       i.Boottime(),
		"brand":          i.Brand(),
		"device_time":    i.DeviceTime(),
		"firmware":       i.Firmware(),
		"mac":            i.Mac(),
		"model":          i.Model(),
		"model_original": i.ModelOriginal(),
		"operation_mode": i.OperationMode(),
		"rebooted":       i.Rebooted(),
		"serial":         i.Serial(),
		"support":        i.Support(),
		"uptime":         i.Uptime(),
		"username":       i.Username(),
		"wifi":           wifiOut,
		"wifi_by_unit":   i.WifiByUnit(),
	}
}
