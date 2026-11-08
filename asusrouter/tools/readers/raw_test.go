package readers

import (
	"reflect"
	"testing"
)

func TestIsTrueInDict(t *testing.T) {
	cases := []struct {
		value  string
		data   map[string]any
		result bool
	}{
		// Value present and equal to 1
		{"key1", map[string]any{"key1": 1}, true},
		{"key1", map[string]any{"key1": "1"}, true},
		{"key1", map[string]any{"key1": true}, true},
		// Value present but not equal to 1
		{"key1", map[string]any{"key1": 0}, false},
		{"key1", map[string]any{"key1": "0"}, false},
		{"key1", map[string]any{"key1": false}, false},
		{"key1", map[string]any{"key1": "some string"}, false},
		// Value not present
		{"key1", map[string]any{"key2": 1}, false},
		{"key1", map[string]any{}, false},
	}
	for _, c := range cases {
		if got := IsTrueInDict(c.value, c.data); got != c.result {
			t.Errorf("IsTrueInDict(%q, %v) = %v", c.value, c.data, got)
		}
	}
}

func TestReadJSONContent(t *testing.T) {
	cases := []struct {
		content  string
		expected map[string]any
	}{
		// JSON from ethernet ports
		{
			`{ "portSpeed": { "WAN 0": "G", "LAN 1": "G", "LAN 2": "G", ` +
				`"LAN 3": "G", "LAN 4": "X", "LAN 5": "X", "LAN 6": "X", ` +
				`"LAN 7": "X", "LAN 8": "G" }, "portCount": { "wanCount": 1, ` +
				`"lanCount": 8 } }`,
			map[string]any{
				"portSpeed": map[string]any{
					"WAN 0": "G", "LAN 1": "G", "LAN 2": "G", "LAN 3": "G", "LAN 4": "X",
					"LAN 5": "X", "LAN 6": "X", "LAN 7": "X", "LAN 8": "G",
				},
				"portCount": map[string]any{"wanCount": int64(1), "lanCount": int64(8)},
			},
		},
		// JSON from sysinfo
		{
			`{"wlc_0_arr":["11", "11", "11"],` +
				`"wlc_1_arr":["2", "2", "2"],` +
				`"wlc_2_arr":["0", "0", "0"],` +
				`"wlc_3_arr":["0", "0", "0"],` +
				`"conn_stats_arr":["394","56"],` +
				`"mem_stats_arr":["882.34", "395.23",` +
				`"0.00", "52.64", "0.00", "0.00", "85343", "7.61 / 63.00 MB"],` +
				`"cpu_stats_arr":["2.18", "2.09", "2.03"]}`,
			map[string]any{
				"wlc_0_arr":      []any{"11", "11", "11"},
				"wlc_1_arr":      []any{"2", "2", "2"},
				"wlc_2_arr":      []any{"0", "0", "0"},
				"wlc_3_arr":      []any{"0", "0", "0"},
				"conn_stats_arr": []any{"394", "56"},
				"mem_stats_arr": []any{
					"882.34", "395.23", "0.00", "52.64", "0.00", "0.00", "85343", "7.61 / 63.00 MB",
				},
				"cpu_stats_arr": []any{"2.18", "2.09", "2.03"},
			},
		},
		// Test valid JSON content
		{`{"key": "value"}`, map[string]any{"key": "value"}},
		// Test empty content (Python None)
		{"", map[string]any{}},
		// Test invalid JSON content
		{"not a json", map[string]any{}},
		// Test missing values
		{`{ , "key1": "value1", , "key2": "value2", }`, map[string]any{"key1": "value1", "key2": "value2"}},
		// Test keys without values
		{
			`{"key1": "value1", "key2": , "key3": "value3", "key4": ,}`,
			map[string]any{"key1": "value1", "key2": nil, "key3": "value3", "key4": nil},
		},
		// Legacy firmware emits get_clientlist without wrapping braces
		{
			"{\n" + `"get_clientlist":"AA:BB":{"name":"x"},` + `"maclist":["AA:BB"],` + "\n" +
				`"get_clientlist_from_json_database":""` + "\n}",
			map[string]any{
				"get_clientlist": map[string]any{
					"AA:BB":   map[string]any{"name": "x"},
					"maclist": []any{"AA:BB"},
				},
				"get_clientlist_from_json_database": "",
			},
		},
		// A properly wrapped get_clientlist is left untouched
		{
			`{"get_clientlist":{"AA:BB":{"name":"x"}},` + `"get_clientlist_from_json_database":""}`,
			map[string]any{
				"get_clientlist":                    map[string]any{"AA:BB": map[string]any{"name": "x"}},
				"get_clientlist_from_json_database": "",
			},
		},
	}
	for _, c := range cases {
		if got := ReadJSONContent(c.content); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("ReadJSONContent(%q) = %#v, want %#v", c.content, got, c.expected)
		}
	}
}

func TestReadJSONContentFail(t *testing.T) {
	// read_json_content returns empty on a non-dict json result
	for _, content := range []string{`"some value"`, "[1, 2]", "42"} {
		if got := ReadJSONContent(content); len(got) != 0 {
			t.Errorf("ReadJSONContent(%q) = %v", content, got)
		}
	}
}

func TestIsRedirectPage(t *testing.T) {
	cases := []struct {
		content  string
		expected bool
	}{
		// An old-firmware bounce page for an unavailable endpoint
		{`<HTML><HEAD><meta http-equiv="refresh" content="0; url=cloud_sync.asp?flag="></HEAD></HTML>`, true},
		// Case and spacing variations still match
		{"<meta HTTP-EQUIV = refresh content='0'>", true},
		// Real payloads are not redirects
		{`{"get_clientlist": {}}`, false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsRedirectPage(c.content); got != c.expected {
			t.Errorf("IsRedirectPage(%q) = %v", c.content, got)
		}
	}
}
