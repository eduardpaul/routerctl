package output

import (
	"strings"
	"testing"
)

func TestTabulate(t *testing.T) {
	r := Tabulate(map[string]any{"b": map[string]any{"x": 1.5, "y": "s"}, "a": map[string]any{"x": int64(2), "z": map[string]any{"k": true}}})
	if strings.Join(r.Header, ",") != "key,x,y,z" || strings.Join(r.Rows[0], ",") != `a,2,,{"k":true}` || strings.Join(r.Rows[1], ",") != "b,1.5,s," {
		t.Errorf("object of objects: %+v", r)
	}
	r = Tabulate(map[string]any{"state": true})
	if strings.Join(r.Header, ",") != "key,value" || strings.Join(r.Rows[0], ",") != "state,true" {
		t.Errorf("object of scalars: %+v", r)
	}
	r = Tabulate([]any{map[string]any{"id": "1"}, map[string]any{"id": "2", "n": nil}})
	if strings.Join(r.Header, ",") != "id,n" || len(r.Rows) != 2 {
		t.Errorf("list: %+v", r)
	}
	if r := Tabulate(false); r.Rows[0][0] != "false" {
		t.Errorf("scalar: %+v", r)
	}
}

func TestWriteFormats(t *testing.T) {
	rows := Rows{Header: []string{"mac", "name"}, Rows: [][]string{{"aa", "phone, mine"}}}
	var b strings.Builder
	if err := Write(&b, CSV, nil, rows); err != nil {
		t.Fatal(err)
	}
	if b.String() != "mac,name\naa,\"phone, mine\"\n" {
		t.Errorf("csv: %q", b.String())
	}
	b.Reset()
	Write(&b, Table, nil, rows)
	if !strings.HasPrefix(b.String(), "MAC  NAME\naa   phone, mine\n") {
		t.Errorf("table: %q", b.String())
	}
	b.Reset()
	Write(&b, JSON, map[string]any{"a": "<b>"}, rows)
	if b.String() != "{\n  \"a\": \"<b>\"\n}\n" {
		t.Errorf("json: %q", b.String())
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("yaml accepted")
	}
}
