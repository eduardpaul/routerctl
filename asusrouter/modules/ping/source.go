package ping

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/poll"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

// Data model

// ARPingResult mirrors `ARPingResult`: the ping result for a single
// target.
type ARPingResult struct {
	Alias     *string                     `json:"alias"`
	Valid     *bool                       `json:"valid"`
	Timestamp *int                        `json:"timestamp"`
	Metrics   map[common.ARMetricType]any `json:"metrics"`
}

// Source

// ARPingSource mirrors `ARPingSource`: the AsusRouter ping data source.
type ARPingSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARPingSource) SourceName() string { return "ARPingSource" }

// ARPingSourceUniversal is the universal instance - preferred.
var ARPingSourceUniversal = &ARPingSource{}

// Fetch

// Diagnostics db holding the last ping run results
const diagDB = "dns_ping"

var diagRequestType = endpoint.GetEndpointRequestType(endpoint.AREndpointFetchDiagnosticsData)

// Poll dns_ping_state until the run finishes before reading results,
// otherwise the router may return empty or partial rows
const pollAttempts = 15

// pollInterval is a variable so tests can shorten it
var pollInterval = 1 * time.Second

// startDelay is the AsyncStartRun delay (<= 0 uses the default)
var startDelay time.Duration

// Diagnostics content columns, in request/response order
var contentColumns = []string{
	"dns_ip",
	"alias",
	"valid",
	"min",
	"avg",
	"max",
	"pkt_sent",
	"pkt_recv",
	"pkt_loss_rate",
	"data_time",
}

var diagContent = strings.Join(contentColumns, ";")

// Column name -> index in the response row
var col = func() map[string]int {
	out := map[string]int{}
	for index, name := range contentColumns {
		out[name] = index
	}
	return out
}()

func rawToFloat(v any) any { return derefFloat(converters.RawToFloat(v)) }
func rawToInt(v any) any   { return derefInt(converters.RawToInt(v)) }
func rawToStr(v any) any   { return derefStr(converters.RawToStr(v)) }
func rawToBool(v any) any  { return derefBool(converters.RawToBool(v)) }

// Metric column name -> (metric, converter); latency stored in ms
// ms as base units for ping
var metricColumns = []struct {
	name      string
	metric    common.ARMetricType
	converter func(any) any
}{
	{"min", common.ARMetricTypeLatencyMin, rawToFloat},
	{"avg", common.ARMetricTypeLatencyAvg, rawToFloat},
	{"max", common.ARMetricTypeLatencyMax, rawToFloat},
	{"pkt_sent", common.ARMetricTypePacketsSent, rawToInt},
	{"pkt_recv", common.ARMetricTypePacketsReceived, rawToInt},
	{"pkt_loss_rate", common.ARMetricTypePacketLoss, rawToFloat},
}

func buildRequest() string {
	// Build the diagnostics request for ping results
	return writers.DictToRequest(
		[]writers.KeyValue{{Key: "db", Value: diagDB}, {Key: "content", Value: diagContent}},
		diagRequestType,
	)
}

func asyncWaitFinished(ctx context.Context, c *registry.Context) (bool, error) {
	// Poll `dns_ping_state` until the ping run reports finished
	probe := func(ctx context.Context) (*string, error) {
		// Force each poll to bypass the state cache, so we see fresh status
		value, err := nvramsource.AsyncGetValue(ctx, c, nvram.ARNvramTypeDnsPingStatus, registry.Kwargs{"force": true})
		if err != nil {
			return nil, err
		}
		return converters.RawToStr(value), nil
	}

	settled := func(raw *string) bool {
		return raw == nil || *raw == "" || ARPingStatusFromValue(*raw) == ARPingStatusFinished
	}

	raw, err := poll.AsyncPollUntil(ctx, probe, settled, pollInterval, pollAttempts)
	if err != nil {
		return false, err
	}
	var value any
	if raw != nil && *raw != nil {
		value = **raw
	}
	return ARPingStatusFromValue(value) == ARPingStatusFinished, nil
}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch ping results; with `refresh`, run a fresh ping first
	if c.FetchDataMulti == nil {
		return map[string]any{}, nil
	}

	// Optionally trigger a fresh run
	if c.Kwargs.Bool("refresh") {
		started, err := action.AsyncStartRun(ctx, c.RunAction, &ARPingAction{}, startDelay)
		if err != nil {
			return nil, err
		}
		if !started {
			return map[string]any{}, nil
		}
	}

	// A run may be in progress (ours or an unrelated trigger); wait it out
	// before reading, or drop stale/partial data
	finished, err := asyncWaitFinished(ctx, c)
	if err != nil {
		return nil, err
	}
	if !finished {
		slog.Debug("Ping run did not finish; dropping results")
		return map[string]any{}, nil
	}

	data, err := c.Read(ctx, endpoint.AREndpointFetchDiagnosticsData, buildRequest())
	if err != nil {
		return nil, err
	}
	m, _ := data.(map[string]any)
	contents, _ := m["contents"].([]any)
	if len(contents) == 0 {
		return map[string]any{}, nil
	}

	return contents, nil
}

func read(row []any, name string, converter func(any) any) any {
	// Convert the named column of a row, or nil if absent
	index := col[name]
	if index < len(row) {
		return converter(row[index])
	}
	return nil
}

func translateRow(row []any) ARPingResult {
	// Translate a single ping row into an `ARPingResult`
	metrics := map[common.ARMetricType]any{}
	for _, mc := range metricColumns {
		if value := read(row, mc.name, mc.converter); value != nil {
			metrics[mc.metric] = value
		}
	}

	result := ARPingResult{Metrics: metrics}
	if v, ok := read(row, "alias", rawToStr).(string); ok {
		result.Alias = &v
	}
	if v, ok := read(row, "valid", rawToBool).(bool); ok {
		result.Valid = &v
	}
	if v, ok := read(row, "data_time", rawToInt).(int); ok {
		result.Timestamp = &v
	}
	return result
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate raw ping rows into per-target results, keyed by IP
	data, ok := raw.([]any)
	if !ok {
		return map[identifiers.IpAddress]ARPingResult{}
	}

	result := map[identifiers.IpAddress]ARPingResult{}
	for _, item := range data {
		row, ok := item.([]any)
		if !ok || len(row) == 0 {
			continue
		}
		ip := identifiers.IpAddressFromValueSafe(row[0])
		if ip == nil {
			continue
		}
		result[*ip] = translateRow(row)
	}

	return result
}

func derefFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func derefInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func derefStr(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func derefBool(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

// Registration

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARPingSource",
		Default:   func() source.DataSource { return &ARPingSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
