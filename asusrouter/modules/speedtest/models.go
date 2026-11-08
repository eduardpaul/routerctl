// Package speedtest mirrors asusrouter/modules/speedtest/.
//
// Python `speedtest/source.py` imports the `history` subpackage, which
// imports this package's models; Go forbids that cycle, so the
// `ARSpeedTestSource` fetcher lives in the `speedtest/source` subpackage.
package speedtest

import (
	"sort"
	"strconv"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/units"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

// Ookla marks a failed run (bad server, disconnect) with this log level
const errorLevel = "error"

// ARSpeedTestServer mirrors `ARSpeedTestServer`: a speedtest server.
type ARSpeedTestServer struct {
	ID       *int    `json:"id"`
	Name     *string `json:"name"`
	Location *string `json:"location"`
	Host     *string `json:"host"`
	Country  *string `json:"country"`
	Port     *int    `json:"port"`
}

// ARSpeedTestResult mirrors `ARSpeedTestResult`: a completed speedtest
// result.
type ARSpeedTestResult struct {
	ResultID  *string                     `json:"result_id"`
	Timestamp *string                     `json:"timestamp"`
	Server    *ARSpeedTestServer          `json:"server"`
	Metrics   map[common.ARMetricType]any `json:"metrics"`
}

// The exe.cgi trigger accepts `type` (empty to run, `list` to fetch
// servers), an optional server `id`, and the `iface` to test over
const (
	ExeTypeRun  = ""
	ExeTypeList = "list"
)

// NormalizeServerID mirrors `normalize_server_id`: coerce a server id to
// its wire string, or nil for the auto server.
func NormalizeServerID(value any) *string {
	parsed := converters.RawToInt(value)
	if parsed == nil || *parsed <= 0 {
		return nil
	}
	s := strconv.Itoa(*parsed)
	return &s
}

// BuildRunRequest mirrors `build_run_request`: the
// `ookla_speedtest_exe.cgi` request body. serverID is an int, a string or
// nil.
func BuildRunRequest(exeType string, serverID any, iface *string) string {
	// Pass id through so a `0` is not dropped; nil becomes an empty field
	if p, ok := serverID.(*string); ok {
		serverID = nil
		if p != nil {
			serverID = *p
		}
	}
	data := []writers.KeyValue{{Key: "type", Value: exeType}, {Key: "id", Value: serverID}}
	if iface != nil && *iface != "" {
		data = append(data, writers.KeyValue{Key: "iface", Value: *iface})
	}
	return writers.DictToRequest(data, consts.RequestTypeGet)
}

// BuildStartTimeRequest mirrors `build_start_time_request`: the
// `set_ookla_speedtest_start_time.cgi` request body.
func BuildStartTimeRequest(timestampMs int64) string {
	return writers.DictToRequest([]writers.KeyValue{{Key: "ookla_start_time", Value: timestampMs}}, consts.RequestTypeGet)
}

// BuildHistoryRequest mirrors `build_history_request`: the
// `ookla_speedtest_write_history.cgi` request body.
//
// Go maps do not keep the device's key order, so each entry is written
// with sorted keys (Python keeps the parsed order); values are rendered
// like Python `json.dumps`.
func BuildHistoryRequest(entries []map[string]any) string {
	body := ""
	for _, entry := range entries {
		body += pyDumps(entry) + "\n"
	}
	return writers.DictToRequest([]writers.KeyValue{{Key: "speedTest_history", Value: body}}, consts.RequestTypeGet)
}

// pyDumps renders a parsed JSON value like Python
// `json.dumps(value, separators=(",", ":"))` (keys sorted, see above).
func pyDumps(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case string:
		return pyJSONString(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return converters.FloatRepr(v)
	case []any:
		out := "["
		for i, item := range v {
			if i > 0 {
				out += ","
			}
			out += pyDumps(item)
		}
		return out + "]"
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := "{"
		for i, k := range keys {
			if i > 0 {
				out += ","
			}
			out += pyJSONString(k) + ":" + pyDumps(v[k])
		}
		return out + "}"
	}
	return pyJSONString(converters.PyStr(value))
}

// pyJSONString renders a JSON string with Python's default
// `ensure_ascii=True` escaping.
func pyJSONString(s string) string {
	const hex = "0123456789abcdef"
	u := func(r rune) string {
		return string([]byte{'\\', 'u', hex[(r>>12)&0xF], hex[(r>>8)&0xF], hex[(r>>4)&0xF], hex[r&0xF]})
	}
	out := []byte{'"'}
	for _, r := range s {
		switch r {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		case '\b':
			out = append(out, '\\', 'b')
		case '\f':
			out = append(out, '\\', 'f')
		default:
			switch {
			case r < 0x20 || (r >= 0x7f && r <= 0xFFFF):
				out = append(out, u(r)...)
			case r > 0xFFFF:
				r -= 0x10000
				out = append(out, u(0xD800+(r>>10))...)
				out = append(out, u(0xDC00+(r&0x3FF))...)
			default:
				out = append(out, byte(r))
			}
		}
	}
	return string(append(out, '"'))
}

// Bandwidth arrives in bytes/s; store rate in bits/s as base units
func bandwidthToBits(value any) *float64 {
	// Convert a bytes/s bandwidth into bits/s, or nil if unparsable
	raw := converters.RawToFloat(value)
	if raw == nil {
		return nil
	}
	bits, err := units.DataRateUnitConverter.ConvertToBase(*raw, units.UnitOfDataRateBytePerSecond)
	if err != nil {
		return nil
	}
	return &bits
}

func serverFromDict(data any) *ARSpeedTestServer {
	// Build a server from a result `server` object
	m, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	return &ARSpeedTestServer{
		ID:       converters.RawToInt(m["id"]),
		Name:     converters.RawToStr(m["name"]),
		Location: converters.RawToStr(m["location"]),
		Host:     converters.RawToStr(m["host"]),
		Country:  converters.RawToStr(m["country"]),
		Port:     converters.RawToInt(m["port"]),
	}
}

// ResultFromDict mirrors `result_from_dict`: a result from a single Ookla
// `result` object.
func ResultFromDict(data map[string]any) ARSpeedTestResult {
	metrics := map[common.ARMetricType]any{}

	if download, ok := data["download"].(map[string]any); ok {
		if value := bandwidthToBits(download["bandwidth"]); value != nil {
			metrics[common.ARMetricTypeDownloadSpeed] = *value
		}
	}

	if upload, ok := data["upload"].(map[string]any); ok {
		if value := bandwidthToBits(upload["bandwidth"]); value != nil {
			metrics[common.ARMetricTypeUploadSpeed] = *value
		}
	}

	if ping, ok := data["ping"].(map[string]any); ok {
		// `low`/`high` bound the latency samples over the run
		for _, item := range []struct {
			key    string
			metric common.ARMetricType
		}{
			{"latency", common.ARMetricTypeLatency},
			{"jitter", common.ARMetricTypeJitter},
			{"low", common.ARMetricTypeLatencyMin},
			{"high", common.ARMetricTypeLatencyMax},
		} {
			if value := converters.RawToFloat(ping[item.key]); value != nil {
				metrics[item.metric] = *value
			}
		}
	}

	if packetLoss := converters.RawToFloat(data["packetLoss"]); packetLoss != nil {
		metrics[common.ARMetricTypePacketLoss] = *packetLoss
	}

	var resultID *string
	if result, ok := data["result"].(map[string]any); ok {
		resultID = converters.RawToStr(result["id"])
	}

	return ARSpeedTestResult{
		ResultID:  resultID,
		Timestamp: converters.RawToStr(data["timestamp"]),
		Server:    serverFromDict(data["server"]),
		Metrics:   metrics,
	}
}

func rowIsError(row any) bool {
	// Whether a single stream row signals a terminal error
	m, ok := row.(map[string]any)
	if !ok {
		return false
	}
	if level, ok := m["level"].(string); ok && level == errorLevel {
		return true
	}
	if _, ok := m["error"]; ok {
		return true
	}
	return ARSpeedTestEventTypeFromValue(m["type"]) == ARSpeedTestEventTypeError
}

// StreamHasError mirrors `stream_has_error`: whether the stream reports a
// terminal error (bad server, disconnect).
func StreamHasError(events any) bool {
	list, ok := events.([]any)
	if !ok {
		return false
	}
	for _, row := range list {
		if rowIsError(row) {
			return true
		}
	}
	return false
}

// StreamErrorMessage mirrors `stream_error_message`: the first error
// message in the stream, if any.
func StreamErrorMessage(events any) *string {
	list, ok := events.([]any)
	if !ok {
		return nil
	}
	for _, row := range list {
		// rowIsError is only true for dict rows
		if rowIsError(row) {
			m := row.(map[string]any)
			message := m["message"]
			if !truthy(message) {
				message = m["error"]
			}
			if message == nil {
				return nil
			}
			return converters.RawToStr(message)
		}
	}
	return nil
}

// ResultServerID mirrors `result_server_id`: the normalized server id of
// a raw result row, or nil.
func ResultServerID(rawResult any) *string {
	m, ok := rawResult.(map[string]any)
	if !ok {
		return nil
	}
	server, ok := m["server"].(map[string]any)
	if !ok {
		return nil
	}
	return NormalizeServerID(server["id"])
}

// RawResultFromEvents mirrors `raw_result_from_events`: the raw final
// `result` row from an Ookla event stream.
func RawResultFromEvents(events any) map[string]any {
	list, ok := events.([]any)
	if !ok {
		return nil
	}

	var final map[string]any
	for _, row := range list {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if ARSpeedTestEventTypeFromValue(m["type"]) == ARSpeedTestEventTypeResult {
			final = m
		}
	}

	return final
}

// ResultFromEvents mirrors `result_from_events`: the final result from an
// Ookla event stream.
func ResultFromEvents(events any) *ARSpeedTestResult {
	final := RawResultFromEvents(events)
	if final == nil {
		return nil
	}
	result := ResultFromDict(final)
	return &result
}

// HistoryFromList mirrors `history_from_list`: a history list from stored
// Ookla result objects.
func HistoryFromList(data any) []ARSpeedTestResult {
	list, ok := data.([]any)
	if !ok {
		return []ARSpeedTestResult{}
	}
	out := []ARSpeedTestResult{}
	for _, row := range list {
		if m, ok := row.(map[string]any); ok && len(m) > 0 {
			out = append(out, ResultFromDict(m))
		}
	}
	return out
}

// ServersFromList mirrors `servers_from_list`: a server list from an
// Ookla server list response.
func ServersFromList(data any) []ARSpeedTestServer {
	list, ok := data.([]any)
	if !ok {
		return []ARSpeedTestServer{}
	}
	out := []ARSpeedTestServer{}
	for _, row := range list {
		if server := serverFromDict(row); server != nil && server.Name != nil {
			out = append(out, *server)
		}
	}
	return out
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
