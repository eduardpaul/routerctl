package routerkit

import (
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
)

var updateSchema = flag.Bool("update", false, "rewrite testdata/model.schema.golden.json")

// schemaOf describes the JSON shape of a type: field name -> type (and
// options), recursively for the contract's own structs.
func schemaOf(t reflect.Type, seen map[reflect.Type]bool) any {
	switch t.Kind() {
	case reflect.Pointer:
		return map[string]any{"ptr": schemaOf(t.Elem(), seen)}
	case reflect.Slice:
		return map[string]any{"list": schemaOf(t.Elem(), seen)}
	case reflect.Map:
		return map[string]any{"map": t.Key().Name(), "of": schemaOf(t.Elem(), seen)}
	case reflect.Struct:
		if t.PkgPath() != reflect.TypeOf(Snapshot{}).PkgPath() || seen[t] {
			return t.String()
		}
		seen[t] = true
		defer delete(seen, t)
		out := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]
			if name == "" {
				name = f.Name
			}
			entry := map[string]any{"type": schemaOf(f.Type, seen)}
			if strings.Contains(tag, "omitempty") {
				entry["omitempty"] = true
			}
			out[name] = entry
		}
		return out
	}
	return t.String()
}

// TestModelSchema: the JSON shape is part of the contract (stdout lines,
// -o json and MQTT payloads are built from it); a rename or a type
// change fails here. Review the diff, then run go test -update.
func TestModelSchema(t *testing.T) {
	types := map[string]any{}
	for _, v := range []any{Snapshot{}, Identity{}, Capabilities{}, DriverInfo{}, Invocation{}, Result{}, OptionSpec{}, Match{}} {
		types[reflect.TypeOf(v).Name()] = schemaOf(reflect.TypeOf(v), map[reflect.Type]bool{})
	}
	got, err := json.MarshalIndent(types, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const path = "testdata/model.schema.golden.json"
	if *updateSchema {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if string(got) != string(want) {
		t.Errorf("the JSON shape of the model changed (bump ContractVersion, review the diff, run go test -update)")
	}
}
