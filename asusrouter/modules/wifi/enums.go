package wifi

// ARWiFiBandFromNband mirrors `ARWiFiBand.from_nband`: map a Broadcom
// `wl{}_nband` radio code to a band.
func ARWiFiBandFromNband(code *string) ARWiFiBand {
	if code == nil || *code == "" {
		return ARWiFiBandUnknown
	}
	if band, ok := nbandToBand[*code]; ok {
		return band
	}
	return ARWiFiBandUnknown
}

// Broadcom `wl{}_nband` radio code -> band, for firmware without
// `WIRELESS_BANDS` (2 = 2.4GHz, 1 = 5GHz, 4 = 6GHz); the first band of
// each frequency is used
var nbandToBand = map[string]ARWiFiBand{
	"1": ARWiFiBandBand5g1,
	"2": ARWiFiBandBand2g1,
	"4": ARWiFiBandBand6g1,
}

// Number of wireless units to probe when deriving bands from per-radio nvram
const ARWifiMaxUnits = 4

// Fallback wireless unit index -> band, for devices with unknown bands
// (`identity.WifiByUnit()` gives the live device-specific map)
var ARWifiUnitFallback = map[int]ARWiFiBand{
	0: ARWiFiBandBand2g1,
	1: ARWiFiBandBand5g1,
	2: ARWiFiBandBand5g2,
	3: ARWiFiBandBand6g1,
}
