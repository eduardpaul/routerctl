// Package systemstatus mirrors asusrouter/modules/system_status/.
package systemstatus

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

// now is the clock used for the diagnostics `ts` (injectable for tests)
var now = time.Now

// Diagnostics window: newest of several points (a single point is
// unreliable for sparse nodes); 60/30 matches the device dashboard
const (
	diagDB       = "sys_detect"
	diagDuration = 60
	diagPoint    = 30
)

var diagRequestType = endpoint.GetEndpointRequestType(endpoint.AREndpointFetchDiagnosticsData)

// Highest per-core index reported; extra cores still feed the aggregate
const maxCores = 8

// legacyPayload is the transient legacy cpu/ram transport from fetchState
// to translate.
type legacyPayload struct {
	now  CpuCounters
	prev CpuCounters
	ram  map[common.ARMetricType]any
}

// Core index -> per-core system type
var coreTypes = func() map[int]ARSystemType {
	out := map[int]ARSystemType{}
	for index := 1; index <= maxCores; index++ {
		out[index] = ARSystemTypeFromValue("CORE_" + strconv.Itoa(index))
	}
	return out
}()

// Modern metric name -> (system type, metric), in response/content order
var metrics = []struct {
	name       string
	systemType ARSystemType
	metric     common.ARMetricType
}{
	{"cpu_usage", ARSystemTypeCpu, common.ARMetricTypeUsage},
	{"mem_usage", ARSystemTypeRam, common.ARMetricTypeUsage},
}

var diagContent = func() string {
	out := ""
	for i, m := range metrics {
		if i > 0 {
			out += ";"
		}
		out += m.name
	}
	return out
}()

// ARSystemStatusSource mirrors `ARSystemStatusSource`: the AsusRouter
// system status data source.
//
// Optionally targets a device by MAC; without a target the main router
// is used. Instances are equal by target MAC. The pipeline caches one
// instance per target, so the cpu counters stashed here survive across
// refreshes and let the legacy usage be derived statelessly.
type ARSystemStatusSource struct {
	target *identifiers.MacAddress

	mu sync.Mutex
	// Previous legacy cpu counters for usage deltas
	prevCPU CpuCounters
}

// NewARSystemStatusSource initializes the source with an optional target
// MAC.
func NewARSystemStatusSource(target any) *ARSystemStatusSource {
	s := &ARSystemStatusSource{}
	s.SetTarget(target)
	return s
}

// SourceName returns the Python class name.
func (*ARSystemStatusSource) SourceName() string { return "ARSystemStatusSource" }

// Target gets the target MAC address.
func (s *ARSystemStatusSource) Target() *identifiers.MacAddress { return s.target }

// SetTarget sets the target MAC address.
func (s *ARSystemStatusSource) SetTarget(value any) {
	s.target = identifiers.MacAddressFromValueSafe(value)
}

// StashCPU stores the current cpu counters, returning the previous ones.
func (s *ARSystemStatusSource) StashCPU(counters CpuCounters) CpuCounters {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.prevCPU
	s.prevCPU = counters
	return prev
}

// SourceKey keys by the target MAC.
func (s *ARSystemStatusSource) SourceKey() []any { return []any{macKey(s.target)} }

func macKey(mac *identifiers.MacAddress) any {
	if mac == nil {
		return nil
	}
	return *mac
}

func macEqual(a, b *identifiers.MacAddress) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ARSystemStatusSourceUniversal is the universal instance - preferred
// (targets the main router).
var ARSystemStatusSourceUniversal = NewARSystemStatusSource(nil)

func forceLegacy(connectionConfig *config.ARConnectionConfig) bool {
	// Whether the connection config forces the legacy data path
	if connectionConfig == nil {
		return false
	}
	value, err := connectionConfig.Lookup(config.ARConnectionConfigKeyForceLegacySystemStatus)
	if err != nil {
		return false
	}
	b, _ := value.(bool)
	return b
}

func buildRequest(mac *identifiers.MacAddress) string {
	// Build the diagnostics request for the given target MAC
	return writers.DictToRequest(
		[]writers.KeyValue{
			{Key: "ts", Value: now().Unix()},
			{Key: "duration", Value: diagDuration},
			{Key: "point", Value: diagPoint},
			{Key: "db", Value: diagDB},
			{Key: "content", Value: diagContent},
			{Key: "filter", Value: "node_mac>txt>" + mac.AsAsus() + ">0"},
		},
		diagRequestType,
	)
}

func getModern(ctx context.Context, c *registry.Context, mac *identifiers.MacAddress) (map[string]any, error) {
	// Fetch modern diagnostics rows for the target, keyed by MAC
	data, err := c.Read(ctx, endpoint.AREndpointFetchDiagnosticsData, buildRequest(mac))
	if err != nil {
		return nil, err
	}
	m, _ := data.(map[string]any)
	contents := m["contents"]
	if !truthy(contents) {
		return map[string]any{}, nil
	}

	return map[string]any{mac.AsAsus(): contents}, nil
}

func getLegacy(ctx context.Context, c *registry.Context, src *ARSystemStatusSource, mac *identifiers.MacAddress, identity *device.ARDeviceIdentity) (map[string]any, error) {
	// Fetch legacy appGet cpu/ram data for the connected router

	// appGet cpu/ram is served only for the connected router, never nodes
	if !macEqual(mac, identityMac(identity)) {
		return map[string]any{}, nil
	}

	data, err := c.Read(ctx, endpoint.AREndpointFetchData, LegacyRequest)
	if err != nil {
		return nil, err
	}
	m, ok := data.(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}

	cpu, _ := m["cpu_usage"].(map[string]any)
	mem, _ := m["memory_usage"].(map[string]any)
	nowCPU := ParseCPU(cpu)
	ram := ParseRAM(mem)
	if len(nowCPU) == 0 && len(ram) == 0 {
		return map[string]any{}, nil
	}

	return map[string]any{mac.AsAsus(): legacyPayload{nowCPU, src.StashCPU(nowCPU), ram}}, nil
}

func identityMac(identity *device.ARDeviceIdentity) *identifiers.MacAddress {
	if identity == nil {
		return nil
	}
	return identity.Mac()
}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch raw cpu/ram data, preferring the modern endpoint.
	//
	// Legacy appGet data is calculated and less precise, so it is used only
	// when the modern endpoint yields nothing or is forced off.
	s := src.(*ARSystemStatusSource)

	mac := s.Target()
	if mac == nil {
		mac = identityMac(c.Identity)
	}
	if mac == nil {
		return map[string]any{}, nil
	}

	if !forceLegacy(c.ConnectionConfig) {
		modern, err := getModern(ctx, c, mac)
		if err != nil {
			return nil, err
		}
		if len(modern) > 0 {
			return modern, nil
		}
	}

	return getLegacy(ctx, c, s, mac, c.Identity)
}

func translateModern(rows []any) map[ARSystemType]map[common.ARMetricType]any {
	// Translate modern diagnostics rows (newest row wins)
	row, _ := rows[len(rows)-1].([]any)
	status := map[ARSystemType]map[common.ARMetricType]any{}
	for index, m := range metrics {
		if index >= len(row) {
			break
		}
		if value := converters.RawToInt(row[index]); value != nil {
			if status[m.systemType] == nil {
				status[m.systemType] = map[common.ARMetricType]any{}
			}
			status[m.systemType][m.metric] = *value
		}
	}

	return status
}

func translateLegacy(payload legacyPayload) map[ARSystemType]map[common.ARMetricType]any {
	// Translate legacy cpu/ram payload into the unified format
	status := map[ARSystemType]map[common.ARMetricType]any{}

	aggregate, cores := TranslateCPU(payload.now, payload.prev)
	if aggregate != nil {
		status[ARSystemTypeCpu] = map[common.ARMetricType]any{common.ARMetricTypeUsage: *aggregate}
		for core, usage := range cores {
			if coreType, ok := coreTypes[core]; ok {
				status[coreType] = map[common.ARMetricType]any{common.ARMetricTypeUsage: usage}
			}
		}
	}

	if len(payload.ram) > 0 {
		ram := make(map[common.ARMetricType]any, len(payload.ram))
		for k, v := range payload.ram {
			ram[k] = v
		}
		status[ARSystemTypeRam] = ram
	}

	return status
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate raw cpu/ram data to the unified per-MAC format
	data, ok := raw.(map[string]any)
	if !ok || len(data) == 0 {
		return map[identifiers.MacAddress]map[ARSystemType]map[common.ARMetricType]any{}
	}

	result := map[identifiers.MacAddress]map[ARSystemType]map[common.ARMetricType]any{}
	for macRaw, payload := range data {
		mac := identifiers.MacAddressFromValueSafe(macRaw)
		if mac == nil || !truthy(payload) {
			continue
		}

		var status map[ARSystemType]map[common.ARMetricType]any
		switch p := payload.(type) {
		case legacyPayload:
			status = translateLegacy(p)
		case []any:
			status = translateModern(p)
		default:
			continue
		}
		if len(status) > 0 {
			result[*mac] = status
		}
	}

	return result
}

// truthy mirrors Python truthiness for parsed router data.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	}
	return true
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARSystemStatusSource",
		Default:   func() source.DataSource { return NewARSystemStatusSource(nil) },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
