package systemstatus

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
)

const kib = 1024

func TestSafeUsage(t *testing.T) {
	tests := []struct {
		used, total, want float64
	}{
		{5, 10, 50.0},   // normal usage
		{10, 10, 100.0}, // normal usage
		{3, 9, 33.33},   // round to 2 decimals
		{-1, 2, 0.0},    // negative usage not allowed
		{1, -2, 0.0},    // negative usage not allowed
		{-1, -1, 100.0}, // both negative values result in positive usage
		{1, 0, 0.0},     // zero total usage not allowed
		{0, 0, 0.0},     // zero total usage not allowed
	}
	for _, tt := range tests {
		if got := safeUsage(tt.used, tt.total); got != tt.want {
			t.Errorf("safeUsage(%v, %v) = %v, want %v", tt.used, tt.total, got, tt.want)
		}
	}
}

func TestSafeUsageHistoric(t *testing.T) {
	tests := []struct {
		used, total, prevUsed, prevTotal, want float64
	}{
		{10, 20, 5, 10, 50.0}, // normal usage
		{10, 20, 10, 20, 0.0}, // no usage
		{6, 18, 3, 9, 33.33},  // round to 2 decimals
		{5, 20, 10, 10, 0.0},  // current used less than previous
		{10, 20, 5, 25, 0.0},  // current total less than previous
		{5, 10, 10, 20, 0.0},  // current values less than previous
	}
	for _, tt := range tests {
		if got := safeUsageHistoric(tt.used, tt.total, tt.prevUsed, tt.prevTotal); got != tt.want {
			t.Errorf("safeUsageHistoric(%v) = %v, want %v", tt, got, tt.want)
		}
	}
}

func TestParseCPU(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want CpuCounters
	}{
		{"each_core", map[string]any{"cpu1_total": "100", "cpu1_usage": "10", "cpu2_total": "200", "cpu2_usage": "20"},
			CpuCounters{1: {100, 10}, 2: {200, 20}}},
		{"stops_at_missing_core", map[string]any{"cpu1_total": "100", "cpu1_usage": "10", "cpu3_total": "300", "cpu3_usage": "30"},
			CpuCounters{1: {100, 10}}},
		{"skips_non_int_core", map[string]any{"cpu1_total": "x", "cpu1_usage": "10"}, CpuCounters{}},
		{"empty", map[string]any{}, CpuCounters{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseCPU(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTranslateCPU(t *testing.T) {
	tests := []struct {
		name      string
		now, prev CpuCounters
		aggregate *float64
		cores     map[int]float64
	}{
		{"no_prev", CpuCounters{1: {100, 10}}, nil, nil, map[int]float64{}},
		{"no_now", CpuCounters{}, CpuCounters{1: {100, 10}}, nil, map[int]float64{}},
		{"aggregate_and_cores", CpuCounters{1: {200, 60}, 2: {200, 40}}, CpuCounters{1: {100, 10}, 2: {100, 20}},
			f(35.0), map[int]float64{1: 50.0, 2: 20.0}},
		{"skips_core_absent_in_prev", CpuCounters{1: {200, 60}, 2: {200, 40}}, CpuCounters{1: {100, 10}},
			f(50.0), map[int]float64{1: 50.0}},
		{"no_overlapping_cores", CpuCounters{1: {200, 60}}, CpuCounters{2: {100, 10}}, nil, map[int]float64{}},
		{"counter_reset_clamps_to_zero", CpuCounters{1: {50, 5}}, CpuCounters{1: {100, 10}},
			f(0.0), map[int]float64{1: 0.0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate, cores := TranslateCPU(tt.now, tt.prev)
			if !reflect.DeepEqual(aggregate, tt.aggregate) || !reflect.DeepEqual(cores, tt.cores) {
				t.Errorf("got (%v, %v), want (%v, %v)", deref(aggregate), cores, deref(tt.aggregate), tt.cores)
			}
		})
	}
}

func TestReadFreeUsed(t *testing.T) {
	tests := []struct {
		name       string
		raw        map[string]any
		free, used *int
	}{
		{"simple", map[string]any{"mem_free": "1", "mem_used": "2", "simple_free": "3", "simple_used": "4"}, i(3), i(4)},
		{"simple_partial", map[string]any{"mem_free": "1", "mem_used": "2", "simple_free": "3"}, i(1), i(2)},
		{"mem_partial", map[string]any{"mem_used": "2"}, nil, i(2)},
		{"empty", map[string]any{}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			free, used := readFreeUsed(tt.raw)
			if !reflect.DeepEqual(free, tt.free) || !reflect.DeepEqual(used, tt.used) {
				t.Errorf("got (%v, %v)", free, used)
			}
		})
	}
}

func TestParseRAM(t *testing.T) {
	ram := ParseRAM(map[string]any{"mem_free": "400", "mem_total": "1000", "mem_used": "600"})
	want := map[common.ARMetricType]any{
		common.ARMetricTypeFree:  400 * kib,
		common.ARMetricTypeTotal: 1000 * kib,
		common.ARMetricTypeUsed:  600 * kib,
		common.ARMetricTypeUsage: 60.0,
	}
	if !reflect.DeepEqual(ram, want) {
		t.Errorf("converts_kib_to_bytes: got %v", ram)
	}

	// Usage needs both used and total
	if ram := ParseRAM(map[string]any{"mem_used": "600"}); !reflect.DeepEqual(ram, map[common.ARMetricType]any{common.ARMetricTypeUsed: 600 * kib}) {
		t.Errorf("no_usage_without_total: got %v", ram)
	}

	// `simple_*` is used only as a complete pair, else `mem_*`
	simple := []struct {
		name       string
		simple     map[string]any
		free, used int
	}{
		{"both", map[string]any{"simple_free": "400", "simple_used": "600"}, 400, 600},
		{"only_used", map[string]any{"simple_used": "600"}, 100, 900},
		{"only_free", map[string]any{"simple_free": "400"}, 100, 900},
		{"empty", map[string]any{"simple_free": "", "simple_used": "600"}, 100, 900},
		{"non_int", map[string]any{"simple_free": "400", "simple_used": "oops"}, 100, 900},
	}
	for _, tt := range simple {
		t.Run(tt.name, func(t *testing.T) {
			raw := map[string]any{"mem_free": "100", "mem_total": "1000", "mem_used": "900"}
			for k, v := range tt.simple {
				raw[k] = v
			}
			ram := ParseRAM(raw)
			want := map[common.ARMetricType]any{
				common.ARMetricTypeFree:  tt.free * kib,
				common.ARMetricTypeTotal: 1000 * kib,
				common.ARMetricTypeUsed:  tt.used * kib,
				common.ARMetricTypeUsage: float64(tt.used) / 10,
			}
			if !reflect.DeepEqual(ram, want) {
				t.Errorf("got %v, want %v", ram, want)
			}
		})
	}

	// Empty or unparseable memory data yields nothing
	for _, raw := range []map[string]any{{}, {"mem_total": "oops"}} {
		if ram := ParseRAM(raw); len(ram) != 0 {
			t.Errorf("unusable %v: got %v", raw, ram)
		}
	}
}

func f(v float64) *float64 { return &v }
func i(v int) *int         { return &v }

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
