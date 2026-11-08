// Legacy appGet cpu/ram parsing for AsusRouter.
//
// CPU values are cumulative tick counters (usage = delta between samples);
// memory values are absolute KiB.

package systemstatus

import (
	"fmt"
	"math"
	"strconv"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/units"
)

// LegacyRequest is the appGet hook request for legacy cpu/ram data
// (connected router only)
var LegacyRequest = endpoint.HookRequest(
	endpoint.ARHookCall{Hook: endpoint.ARHookCpuUsage, Args: "appobj"},
	endpoint.ARHookCall{Hook: endpoint.ARHookMemoryUsage, Args: "appobj"},
)

// Memory values are reported in KiB; convert them to bytes
var kibToBytes = func() func(float64) float64 {
	convert, err := units.DataUnitConverter.ConverterFactory(units.UnitOfDataKibibyte, units.UnitOfDataByte)
	if err != nil {
		panic(err)
	}
	return convert
}()

// Raw memory (free, used) keys in preference order: Web-GUI shows
// simple_* (excludes buffers/cache), older FW lacks it
var ramFreeUsed = [][2]string{
	{"simple_free", "simple_used"},
	{"mem_free", "mem_used"},
}

const ramTotal = "mem_total"

// CpuCounters holds cumulative cpu tick counters per core:
// {core_index: (total, used)}
type CpuCounters map[int][2]int

// pyRound mirrors Python `round(value, digits)` (correctly rounded,
// ties to even on the exact binary value).
func pyRound(value float64, digits int) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	r, err := strconv.ParseFloat(strconv.FormatFloat(value, 'f', digits, 64), 64)
	if err != nil {
		return value
	}
	return r
}

func safeUsage(used, total float64) float64 {
	// Calculate usage in percents, clamping negatives to 0.0
	if total == 0 {
		return 0.0
	}

	usage := pyRound(used/total*100, 2)

	// Don't allow negative usage
	if usage < 0 {
		return 0.0
	}

	return usage
}

func safeUsageHistoric(used, total, prevUsed, prevTotal float64) float64 {
	// Calculate usage in percents for the difference between two samples
	usedDiff := used - prevUsed
	totalDiff := total - prevTotal

	// Don't allow negative differences
	if usedDiff < 0 || totalDiff < 0 {
		return 0.0
	}

	return safeUsage(usedDiff, totalDiff)
}

// ParseCPU mirrors `parse_cpu`: cumulative cpu tick counters per core.
func ParseCPU(raw map[string]any) CpuCounters {
	counters := CpuCounters{}
	for core := 1; ; core++ {
		totalKey := fmt.Sprintf("cpu%d_total", core)
		rawTotal, ok := raw[totalKey]
		if !ok {
			break
		}
		total := converters.RawToInt(rawTotal)
		used := converters.RawToInt(raw[fmt.Sprintf("cpu%d_usage", core)])
		if total != nil && used != nil {
			counters[core] = [2]int{*total, *used}
		}
	}

	return counters
}

// TranslateCPU mirrors `translate_cpu`: aggregate and per-core usage from
// counter deltas. A previous sample is required; without it no usage is
// produced.
func TranslateCPU(now, prev CpuCounters) (*float64, map[int]float64) {
	if len(now) == 0 || len(prev) == 0 {
		return nil, map[int]float64{}
	}

	cores := map[int]float64{}
	var aggTotal, aggUsed, aggPrevTotal, aggPrevUsed int
	for core, sample := range now {
		previous, ok := prev[core]
		if !ok {
			continue
		}
		total, used := sample[0], sample[1]
		prevTotal, prevUsed := previous[0], previous[1]
		cores[core] = safeUsageHistoric(float64(used), float64(total), float64(prevUsed), float64(prevTotal))
		aggTotal += total
		aggUsed += used
		aggPrevTotal += prevTotal
		aggPrevUsed += prevUsed
	}

	if len(cores) == 0 {
		return nil, map[int]float64{}
	}

	aggregate := safeUsageHistoric(
		float64(aggUsed), float64(aggTotal), float64(aggPrevUsed), float64(aggPrevTotal),
	)

	return &aggregate, cores
}

func readFreeUsed(raw map[string]any) (*int, *int) {
	// Read the first complete (free, used) pair, else the last partial
	var free, used *int
	for _, keys := range ramFreeUsed {
		free = converters.RawToInt(raw[keys[0]])
		used = converters.RawToInt(raw[keys[1]])
		if free != nil && used != nil {
			break
		}
	}

	return free, used
}

// ParseRAM mirrors `parse_ram`: memory data, sizes converted from KiB to
// bytes.
func ParseRAM(raw map[string]any) map[common.ARMetricType]any {
	free, used := readFreeUsed(raw)
	total := converters.RawToInt(raw[ramTotal])

	sizes := []struct {
		metric common.ARMetricType
		value  *int
	}{
		{common.ARMetricTypeFree, free},
		{common.ARMetricTypeTotal, total},
		{common.ARMetricTypeUsed, used},
	}
	ram := map[common.ARMetricType]any{}
	for _, size := range sizes {
		if size.value != nil {
			ram[size.metric] = int(kibToBytes(float64(*size.value)))
		}
	}
	if used != nil && total != nil {
		ram[common.ARMetricTypeUsage] = safeUsage(float64(*used), float64(*total))
	}

	return ram
}
