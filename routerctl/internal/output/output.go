// Package output renders command results as a table, JSON or CSV.
package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// Format is an output format.
type Format string

// Formats.
const (
	Table Format = "table"
	JSON  Format = "json"
	CSV   Format = "csv"
)

// ParseFormat validates a format name.
func ParseFormat(s string) (Format, error) {
	switch Format(strings.ToLower(s)) {
	case Table, "":
		return Table, nil
	case JSON:
		return JSON, nil
	case CSV:
		return CSV, nil
	}
	return "", fmt.Errorf("unknown output format %q (table, json, csv)", s)
}

// Rows is a header plus rows of cells.
type Rows struct {
	Header []string
	Rows   [][]string
}

// WriteJSON writes indented JSON.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Write renders v: JSON as is, table / CSV from rows.
func Write(w io.Writer, f Format, v any, rows Rows) error {
	switch f {
	case JSON:
		return WriteJSON(w, v)
	case CSV:
		cw := csv.NewWriter(w)
		if err := cw.Write(rows.Header); err != nil {
			return err
		}
		if err := cw.WriteAll(rows.Rows); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	default:
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		upper := make([]string, len(rows.Header))
		for i, h := range rows.Header {
			upper[i] = strings.ToUpper(h)
		}
		fmt.Fprintln(tw, strings.Join(upper, "\t"))
		for _, r := range rows.Rows {
			cells := make([]string, len(r))
			for i, c := range r {
				cells[i] = strings.NewReplacer("\t", " ", "\n", " ").Replace(c)
			}
			fmt.Fprintln(tw, strings.Join(cells, "\t"))
		}
		return tw.Flush()
	}
}

// Cell renders one value for a table / CSV cell.
func Cell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case *int:
		if t == nil {
			return ""
		}
		return strconv.Itoa(*t)
	case *float64:
		if t == nil {
			return ""
		}
		return strconv.FormatFloat(*t, 'f', -1, 64)
	case *bool:
		if t == nil {
			return ""
		}
		return strconv.FormatBool(*t)
	case *int64:
		if t == nil {
			return ""
		}
		return strconv.FormatInt(*t, 10)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Tabulate turns a generic JSON-like tree into rows:
//   - a list of objects: one row per object, columns = union of keys
//   - an object of objects: one row per entry ("key" first)
//   - an object of scalars: key / value rows
//   - anything else: one "value" cell
func Tabulate(v any) Rows {
	switch t := v.(type) {
	case []any:
		var objs []map[string]any
		for _, x := range t {
			m, ok := x.(map[string]any)
			if !ok {
				rows := Rows{Header: []string{"value"}}
				for _, y := range t {
					rows.Rows = append(rows.Rows, []string{Cell(y)})
				}
				return rows
			}
			objs = append(objs, m)
		}
		return objectRows(nil, objs)
	case map[string]any:
		allObj := len(t) > 0
		for _, x := range t {
			if _, ok := x.(map[string]any); !ok {
				allObj = false
			}
		}
		keys := sortedKeys(t)
		if allObj {
			objs := make([]map[string]any, len(keys))
			for i, k := range keys {
				objs[i] = t[k].(map[string]any)
			}
			return objectRows(keys, objs)
		}
		rows := Rows{Header: []string{"key", "value"}}
		for _, k := range keys {
			rows.Rows = append(rows.Rows, []string{k, Cell(t[k])})
		}
		return rows
	}
	return Rows{Header: []string{"value"}, Rows: [][]string{{Cell(v)}}}
}

func objectRows(keys []string, objs []map[string]any) Rows {
	cols := map[string]bool{}
	for _, o := range objs {
		for k := range o {
			cols[k] = true
		}
	}
	var header []string
	for k := range cols {
		header = append(header, k)
	}
	sort.Strings(header)
	rows := Rows{}
	if keys != nil {
		rows.Header = append([]string{"key"}, header...)
	} else {
		rows.Header = header
	}
	for i, o := range objs {
		var r []string
		if keys != nil {
			r = append(r, keys[i])
		}
		for _, h := range header {
			r = append(r, Cell(o[h]))
		}
		rows.Rows = append(rows.Rows, r)
	}
	return rows
}
