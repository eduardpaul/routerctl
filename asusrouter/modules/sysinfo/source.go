// Package sysinfo mirrors asusrouter/modules/sysinfo/.
package sysinfo

// SysInfo data source for AsusRouter (Merlin firmware only).

import (
	"context"
	"math"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/units"
)

// Merlin reports memory and JFFS in decimal MB; NVRAM in bytes
var megabytesToBytes = func() func(float64) float64 {
	convert, err := units.DataUnitConverter.ConverterFactory(units.UnitOfDataMegabyte, units.UnitOfDataByte)
	if err != nil {
		panic(err)
	}
	return convert
}()

// Load average windows in minutes, in `cpu_stats_arr` order
var loadAverageWindows = []int{1, 5, 15}

// Client counts in `wlc_{i}_arr` order
var wlanClientOrder = []ARWlanClientCount{
	ARWlanClientCountAssociated,
	ARWlanClientCountAuthorized,
	ARWlanClientCountAuthenticated,
}

// Connection counts in `conn_stats_arr` order
var connectionOrder = []common.ARMetricType{
	common.ARMetricTypeTotal,
	common.ARMetricTypeActive,
}

// MB memory fields by `mem_stats_arr` index; USED/AVAILABLE only from 388.7
var memoryMBFields = []indexed[ARMemoryType]{
	{0, ARMemoryTypeTotal},
	{1, ARMemoryTypeFree},
	{2, ARMemoryTypeBuffers},
	{3, ARMemoryTypeCache},
	{4, ARMemoryTypeSwapUsed},
	{5, ARMemoryTypeSwapTotal},
	{8, ARMemoryTypeUsed},
	{9, ARMemoryTypeAvailable},
}

// Index of the NVRAM value (already in bytes) in `mem_stats_arr`
const memoryNvramIndex = 6

// Index of the JFFS value in `mem_stats_arr`
const memoryJffsIndex = 7

// indexed is one `(index, key)` pair of a positional map.
type indexed[K comparable] struct {
	index int
	key   K
}

// enumerate mirrors Python `enumerate(keys)`.
func enumerate[K comparable](keys []K) []indexed[K] {
	out := make([]indexed[K], len(keys))
	for i, k := range keys {
		out[i] = indexed[K]{i, k}
	}
	return out
}

// mbToBytes converts a raw MB value to bytes, or nil.
func mbToBytes(value any) *int {
	megabytes := converters.RawToFloat(value)
	if megabytes == nil {
		return nil
	}
	// Python round() rounds half to even
	b := int(math.RoundToEven(megabytesToBytes(*megabytes)))
	return &b
}

// sequence reads a raw value the way Python indexes it (a list, or the
// characters of a str); nil when it cannot be indexed.
func sequence(raw any) []any {
	switch v := raw.(type) {
	case []any:
		return v
	case string:
		out := make([]any, 0, len(v))
		for _, r := range v {
			out = append(out, string(r))
		}
		return out
	}
	return nil
}

// mapPositional maps raw values at given indices onto keys, skipping
// gaps and None.
func mapPositional[K comparable, V any](raw []any, fields []indexed[K], convert func(any) *V) map[K]V {
	result := map[K]V{}
	for _, f := range fields {
		if f.index >= len(raw) {
			continue
		}
		if value := convert(raw[f.index]); value != nil {
			result[f.key] = *value
		}
	}

	return result
}

// wlanIndexToBand resolves `wlc` index -> band, preferring
// `identity.wifi`.
func wlanIndexToBand(identity *device.ARDeviceIdentity) map[int]wifi.ARWiFiBand {
	if identity != nil && len(identity.Wifi()) > 0 {
		return identity.WifiByUnit()
	}
	return wifi.ARWifiUnitFallback
}

// translateWlan translates per-band wireless client counts.
func translateWlan(data map[string]any, indexToBand map[int]wifi.ARWiFiBand) map[wifi.ARWiFiBand]map[ARWlanClientCount]int {
	wlan := map[wifi.ARWiFiBand]map[ARWlanClientCount]int{}

	for index := 0; ; index++ {
		raw, ok := data["wlc_"+converters.PyStr(index)+"_arr"]
		if !ok || raw == nil {
			break
		}
		band, ok := indexToBand[index]
		if !ok {
			band = wifi.ARWiFiBandUnknown
		}
		wlan[band] = mapPositional(sequence(raw), enumerate(wlanClientOrder), converters.RawToInt)
	}

	return wlan
}

// orEmpty mirrors `data.get(key) or ()`.
func orEmpty(data map[string]any, key string) []any {
	return sequence(data[key])
}

// translateConnections translates total and active connection counts.
func translateConnections(data map[string]any) map[common.ARMetricType]int {
	raw := orEmpty(data, "conn_stats_arr")
	return mapPositional(raw, enumerate(connectionOrder), converters.RawToInt)
}

// translateJffs translates the JFFS value in both reporting formats.
func translateJffs(value string) map[ARMemoryType]int {
	jffs := map[ARMemoryType]int{}

	// Before 388.7: `XX.xx / YY.yy MB` (used / total)
	usedStr, totalStr, sep := strings.Cut(value, "/")
	if sep {
		used := mbToBytes(usedStr)
		total := mbToBytes(strings.TrimSuffix(totalStr, " MB"))
		if used != nil {
			jffs[ARMemoryTypeJffsUsed] = *used
		}
		if total != nil {
			jffs[ARMemoryTypeJffsTotal] = *total
		}
		if used != nil && total != nil {
			jffs[ARMemoryTypeJffsFree] = *total - *used
		}
		return jffs
	}

	// From 388.7: a single `free` value
	if free := mbToBytes(value); free != nil {
		jffs[ARMemoryTypeJffsFree] = *free
	}
	return jffs
}

// translateMemory translates memory values into bytes.
func translateMemory(data map[string]any) map[ARMemoryType]int {
	raw := orEmpty(data, "mem_stats_arr")
	memory := mapPositional(raw, memoryMBFields, mbToBytes)

	if len(raw) > memoryNvramIndex {
		if nvram := converters.RawToInt(raw[memoryNvramIndex]); nvram != nil {
			memory[ARMemoryTypeNvram] = *nvram
		}
	}

	if len(raw) > memoryJffsIndex {
		// Python calls str methods on the value; a non-str one has none
		if value, ok := raw[memoryJffsIndex].(string); ok {
			for k, v := range translateJffs(value) {
				memory[k] = v
			}
		}
	}

	return memory
}

// translateLoadAverage translates the 1/5/15-minute load averages.
func translateLoadAverage(data map[string]any) map[int]float64 {
	raw := orEmpty(data, "cpu_stats_arr")
	return mapPositional(raw, enumerate(loadAverageWindows), converters.RawToFloat)
}

// ARSysInfoSource mirrors `ARSysInfoSource`: SysInfo data source for the
// connected router (Merlin firmware).
type ARSysInfoSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARSysInfoSource) SourceName() string { return "ARSysInfoSource" }

// ARSysInfoSourceUniversal is the universal instance - preferred.
var ARSysInfoSourceUniversal = &ARSysInfoSource{}

// fetchState mirrors `fetch_state`: fetch the raw sysinfo JS variables.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	response, err := c.Read(ctx, endpoint.AREndpointFetchSysinfo, "")
	if err != nil {
		return nil, err
	}

	if m, ok := response.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// translateState mirrors `translate_state`: translate raw sysinfo
// variables into typed data categories.
func translateState(raw any, tc *registry.TranslateContext) any {
	result := map[ARSysInfoType]any{}
	data, _ := raw.(map[string]any)

	var identity *device.ARDeviceIdentity
	if tc != nil {
		identity = tc.Identity
	}

	if wlan := translateWlan(data, wlanIndexToBand(identity)); len(wlan) > 0 {
		result[ARSysInfoTypeWlan] = wlan
	}

	if connections := translateConnections(data); len(connections) > 0 {
		result[ARSysInfoTypeConnections] = connections
	}

	if memory := translateMemory(data); len(memory) > 0 {
		result[ARSysInfoTypeMemory] = memory
	}

	if loadAverage := translateLoadAverage(data); len(loadAverage) > 0 {
		result[ARSysInfoTypeLoadAverage] = loadAverage
	}

	return result
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARSysInfoSource",
		Default:   func() source.DataSource { return &ARSysInfoSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
