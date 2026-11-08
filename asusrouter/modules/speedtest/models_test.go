package speedtest

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
)

// A single finished result row, mirroring the Ookla `result` event shape
func resultRow() map[string]any {
	return map[string]any{
		"type":       "result",
		"timestamp":  "2026-01-01T00:00:00Z",
		"ping":       map[string]any{"latency": 1.5, "jitter": 0.2, "low": 1.0, "high": 2.0},
		"download":   map[string]any{"bandwidth": int64(1000), "bytes": int64(1), "servers": []any{map[string]any{"id": int64(1)}}},
		"upload":     map[string]any{"bandwidth": int64(500)},
		"packetLoss": int64(0),
		"server": map[string]any{
			"id":       int64(23314),
			"host":     "speedtest.example",
			"port":     int64(8080),
			"name":     "Example Inc.",
			"location": "Somewhere",
			"country":  "Neverland",
		},
		"result": map[string]any{"id": "abc-123", "persisted": true},
	}
}

func withResultID(id string) map[string]any {
	row := resultRow()
	row["result"] = map[string]any{"id": id}
	return row
}

func sp(s string) *string { return &s }
func ip(i int) *int       { return &i }

func TestEnums(t *testing.T) {
	if ARSpeedTestStateFromValue("1") != ARSpeedTestStateRunning || ARSpeedTestStateFromValue("0") != ARSpeedTestStateIdle ||
		ARSpeedTestStateFromValue("9") != ARSpeedTestStateUnknown {
		t.Error("state")
	}
	if ARSpeedTestEventTypeFromValue("result") != ARSpeedTestEventTypeResult || ARSpeedTestEventTypeFromValue("nope") != ARSpeedTestEventTypeUnknown {
		t.Error("event type")
	}
}

func TestBuildRunRequest(t *testing.T) {
	tests := []struct {
		name     string
		exeType  string
		serverID any
		iface    *string
		want     string
	}{
		{"default_start", ExeTypeRun, nil, nil, "type=&id="},
		{"with_server", ExeTypeRun, "42", nil, "type=&id=42"},
		{"with_integer_server", ExeTypeRun, 54112, nil, "type=&id=54112"},
		{"server_id_zero_not_dropped", ExeTypeRun, 0, nil, "type=&id=0"},
		{"with_iface", ExeTypeRun, nil, sp("tun11"), "type=&id=&iface=tun11"},
		{"list", ExeTypeList, nil, nil, "type=list&id="},
		{"nil_pointer_server", ExeTypeRun, (*string)(nil), nil, "type=&id="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildRunRequest(tt.exeType, tt.serverID, tt.iface); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	if got := BuildStartTimeRequest(1783106179479); got != "ookla_start_time=1783106179479" {
		t.Errorf("start time: %q", got)
	}
}

func TestResultFromDict(t *testing.T) {
	want := ARSpeedTestResult{
		ResultID:  sp("abc-123"),
		Timestamp: sp("2026-01-01T00:00:00Z"),
		Server: &ARSpeedTestServer{
			ID: ip(23314), Name: sp("Example Inc."), Location: sp("Somewhere"),
			Host: sp("speedtest.example"), Country: sp("Neverland"), Port: ip(8080),
		},
		Metrics: map[common.ARMetricType]any{
			common.ARMetricTypeDownloadSpeed: 8000.0,
			common.ARMetricTypeUploadSpeed:   4000.0,
			common.ARMetricTypeLatency:       1.5,
			common.ARMetricTypeJitter:        0.2,
			common.ARMetricTypeLatencyMin:    1.0,
			common.ARMetricTypeLatencyMax:    2.0,
			common.ARMetricTypePacketLoss:    0.0,
		},
	}
	if got := ResultFromDict(resultRow()); !reflect.DeepEqual(got, want) {
		t.Errorf("full: got %+v", got)
	}

	if got := ResultFromDict(map[string]any{}); !reflect.DeepEqual(got, ARSpeedTestResult{Metrics: map[common.ARMetricType]any{}}) {
		t.Errorf("minimal: %+v", got)
	}

	got := ResultFromDict(map[string]any{"download": "x", "upload": nil, "ping": 1, "result": "y", "server": nil})
	if len(got.Metrics) != 0 || got.ResultID != nil || got.Server != nil {
		t.Errorf("non_dict_sections_ignored: %+v", got)
	}

	got = ResultFromDict(map[string]any{"download": map[string]any{"bandwidth": "n/a"}})
	if _, ok := got.Metrics[common.ARMetricTypeDownloadSpeed]; ok {
		t.Error("unparsable bandwidth kept")
	}
}

func TestResultFromEvents(t *testing.T) {
	if ResultFromEvents("nope") != nil {
		t.Error("non_list")
	}
	if ResultFromEvents([]any{map[string]any{"type": "download"}, map[string]any{"type": "ping"}}) != nil {
		t.Error("no_result_row")
	}
	events := []any{withResultID("first"), "noise", map[string]any{"type": "download"}, withResultID("last"), map[string]any{}}
	if r := ResultFromEvents(events); r == nil || *r.ResultID != "last" {
		t.Errorf("picks_last_result: %+v", r)
	}
}

func TestRawResultFromEvents(t *testing.T) {
	if RawResultFromEvents("nope") != nil {
		t.Error("non_list")
	}
	if RawResultFromEvents([]any{map[string]any{"type": "download"}, map[string]any{}}) != nil {
		t.Error("no_result_row")
	}
	last := withResultID("last")
	got := RawResultFromEvents([]any{resultRow(), map[string]any{"type": "upload"}, last, map[string]any{}})
	if !reflect.DeepEqual(got, last) {
		t.Errorf("returns_last_raw_row: %v", got)
	}
}

func TestStreamError(t *testing.T) {
	// The real Ookla error row for a bad server id
	errorRow := map[string]any{
		"type":      "log",
		"timestamp": "2026-07-03T20:20:25Z",
		"message":   "Configuration - No servers defined (NoServersException)",
		"level":     "error",
	}
	empty := map[string]any{}

	if !StreamHasError([]any{errorRow, empty}) {
		t.Error("detects_error_level_log")
	}
	if m := StreamErrorMessage([]any{errorRow, empty}); m == nil || *m != errorRow["message"] {
		t.Errorf("message_returned: %v", m)
	}
	rows := []any{map[string]any{"error": "boom"}, empty}
	if m := StreamErrorMessage(rows); !StreamHasError(rows) || m == nil || *m != "boom" {
		t.Error("error_field")
	}
	if !StreamHasError([]any{map[string]any{"type": "error"}}) {
		t.Error("error_type")
	}
	if StreamHasError([]any{map[string]any{"type": "log", "level": "info"}}) {
		t.Error("benign_log_not_error")
	}
	if StreamHasError([]any{map[string]any{"type": "result"}, empty}) {
		t.Error("normal_stream_not_error")
	}
	if StreamHasError("nope") || StreamErrorMessage("nope") != nil {
		t.Error("non_list")
	}
	if StreamHasError([]any{"x", empty}) {
		t.Error("non_dict_rows_ignored")
	}
	if StreamErrorMessage([]any{map[string]any{"type": "result"}}) != nil {
		t.Error("message_none_when_no_error")
	}
	if StreamErrorMessage([]any{map[string]any{"type": "error"}}) != nil {
		t.Error("error_without_message")
	}
}

func TestResultServerID(t *testing.T) {
	if id := ResultServerID(map[string]any{"server": map[string]any{"id": int64(54112)}}); id == nil || *id != "54112" {
		t.Errorf("reads_and_normalizes: %v", id)
	}
	if ResultServerID(map[string]any{"result": map[string]any{"id": "x"}}) != nil || ResultServerID("nope") != nil {
		t.Error("no server / non dict")
	}
}

func TestNormalizeServerID(t *testing.T) {
	if id := NormalizeServerID(54112); id == nil || *id != "54112" {
		t.Error("int")
	}
	if id := NormalizeServerID("54112"); id == nil || *id != "54112" {
		t.Error("str")
	}
	for _, value := range []any{nil, 0, -5, "0", "abc", ""} {
		if NormalizeServerID(value) != nil {
			t.Errorf("%v falls back to auto", value)
		}
	}
}

func TestBuildHistoryRequest(t *testing.T) {
	entries := []map[string]any{{"result": map[string]any{"id": "a"}}, {"result": map[string]any{"id": "b"}}}
	request := BuildHistoryRequest(entries)
	if !strings.HasPrefix(request, "speedTest_history=") {
		t.Fatalf("request %q", request)
	}
	body, err := url.QueryUnescape(strings.TrimPrefix(request, "speedTest_history="))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		var entry map[string]map[string]string
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, entry["result"]["id"])
	}
	if !reflect.DeepEqual(ids, []string{"a", "b"}) {
		t.Errorf("ids %v", ids)
	}
}

func TestPyDumps(t *testing.T) {
	got := pyDumps(map[string]any{"b": []any{1.5, 0.0, int64(3), true, nil}, "a": "é<\"x\">"})
	want := "{\"a\":\"\\u00e9<\\\"x\\\">\",\"b\":[1.5,0.0,3,true,null]}"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestHistoryFromList(t *testing.T) {
	if got := HistoryFromList(nil); len(got) != 0 {
		t.Error("non_list")
	}
	if got := HistoryFromList([]any{resultRow(), map[string]any{}, "x", resultRow()}); len(got) != 2 {
		t.Errorf("skips_empty_and_non_dict: %d", len(got))
	}
}

func TestServersFromList(t *testing.T) {
	if got := ServersFromList(nil); len(got) != 0 {
		t.Error("non_list")
	}
	servers := ServersFromList([]any{
		map[string]any{"id": int64(1), "name": "A", "location": "L"},
		map[string]any{"id": int64(2)},
		map[string]any{},
		"x",
	})
	if len(servers) != 1 || *servers[0].Name != "A" {
		t.Errorf("skips_nameless_and_empty: %+v", servers)
	}
}
