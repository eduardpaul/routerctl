package writers

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func TestDictToRequest(t *testing.T) {
	mac, _ := identifiers.MacAddressFromValue("aa:bb:cc:dd:ee:ff")
	post, get := consts.RequestTypePost, consts.RequestTypeGet
	cases := []struct {
		id          string
		data        []KeyValue
		requestType consts.RequestType
		expected    string
	}{
		{"post-simple-single", []KeyValue{{"key", "value"}}, post, "'key':'value'"},
		{"get-simple-single", []KeyValue{{"key", "value"}}, get, "key=value"},
		{"post-simple-multiple", []KeyValue{{"key1", "value1"}, {"key2", "value2"}}, post, "'key1':'value1';'key2':'value2'"},
		{"get-simple-multiple", []KeyValue{{"key1", "value1"}, {"key2", "value2"}}, get, "key1=value1&key2=value2"},
		{"empty-post", nil, post, ""},
		{"empty-get", []KeyValue{}, get, ""},
		{"MacAddress", []KeyValue{{"mac", mac}}, post, "'mac':'AA:BB:CC:DD:EE:FF'"},
		{"bool-post", []KeyValue{{"key1", true}, {"key2", false}}, post, "'key1':'1';'key2':'0'"},
		{"get-quoting-spaces", []KeyValue{{"a b", "c d"}}, get, "a+b=c+d"},
		{"get-quoting-special-chars", []KeyValue{{"sym", "&=+?"}}, get, "sym=%26%3D%2B%3F"},
		{"get-bool", []KeyValue{{"flag", true}}, get, "flag=1"},
		{"post-escape-single-quotes", []KeyValue{{"k'", "v'"}}, post, `'k\'':'v\''`},
		// Go-only checks
		{"get-unicode-and-safe", []KeyValue{{"x", "é/~_.-"}}, get, "x=%C3%A9%2F~_.-"},
		{"post-none-and-int", []KeyValue{{"a", nil}, {"b", 5}}, post, "'a':'';'b':'5'"},
	}
	for _, c := range cases {
		if got := DictToRequest(c.data, c.requestType); got != c.expected {
			t.Errorf("%s: got %q, want %q", c.id, got, c.expected)
		}
	}
}

func TestMapToRequest(t *testing.T) {
	got := MapToRequest(map[string]any{"b": "2", "a": "1"}, consts.RequestTypeGet)
	if got != "a=1&b=2" {
		t.Fatalf("got %q", got)
	}
}
