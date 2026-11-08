package zte

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Instance is one object instance: the ParaName/ParaValue pairs of an
// <Instance> element (or the scalar fields of a JSON object).
type Instance map[string]string

// Str returns a trimmed value ("" when missing).
func (in Instance) Str(name string) string { return strings.TrimSpace(in[name]) }

// Has reports whether a parameter is present.
func (in Instance) Has(name string) bool {
	_, ok := in[name]
	return ok
}

// Bool reports whether the value is "1" or "true".
func (in Instance) Bool(name string) bool {
	switch strings.ToLower(in.Str(name)) {
	case "1", "true", "yes", "on", "enable", "enabled":
		return true
	}
	return false
}

// Int parses an integer value.
func (in Instance) Int(name string) (int64, bool) {
	v, err := strconv.ParseInt(in.Str(name), 10, 64)
	return v, err == nil
}

// Uint parses an unsigned integer value.
func (in Instance) Uint(name string) (uint64, bool) {
	v, err := strconv.ParseUint(in.Str(name), 10, 64)
	return v, err == nil
}

// Float parses the leading number of a value, ignoring a trailing unit
// ("-21.3 dBm", "62%", "45 ℃").
func (in Instance) Float(name string) (float64, bool) { return leadingFloat(in.Str(name)) }

// Response is one parsed data endpoint response.
//
// XML responses (<ajax_response_xml_root>) are mapped as: every child
// element that holds <Instance> elements becomes Objects[name]; every
// other child element becomes Values[name] (IF_ERRORSTR, encode, ...).
// JSON responses (some tags, e.g. topo_lua.lua) are mapped the same way:
// objects with an "Instance" array or with scalar fields become Objects,
// top-level scalars become Values.
type Response struct {
	Tag     string                `json:"tag,omitempty"`
	Objects map[string][]Instance `json:"objects"`
	Values  map[string]string     `json:"values,omitempty"`
	// Encoded lists the parameter names the device marked as encrypted
	// (<encode>Pwd</encode>); see Client.DecodeValue
	Encoded []string `json:"encoded,omitempty"`
}

// Object returns the instances of an object (nil when missing).
func (r *Response) Object(name string) []Instance {
	if r == nil {
		return nil
	}
	return r.Objects[name]
}

// First returns the first instance of an object (an empty Instance when
// missing, so lookups are always safe).
func (r *Response) First(name string) Instance {
	if insts := r.Object(name); len(insts) > 0 {
		return insts[0]
	}
	return Instance{}
}

// ErrorStr is the IF_ERRORSTR value ("SUCC", "SessionTimeout", ...).
func (r *Response) ErrorStr() string { return strings.TrimSpace(r.Values["IF_ERRORSTR"]) }

// ObjectNames lists the object names, sorted.
func (r *Response) ObjectNames() []string {
	out := make([]string, 0, len(r.Objects))
	for k := range r.Objects {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ErrParse is wrapped by parse failures.
var ErrParse = errors.New("zte: cannot parse response")

// Parse parses a data endpoint body (XML or JSON). It is tolerant:
// unknown elements are kept, a ParaName without a ParaValue maps to "",
// HTML entities are decoded, and a truncated document returns what was
// read so far when the root element was seen.
func Parse(body []byte) (*Response, error) {
	trim := bytes.TrimSpace(bytes.TrimPrefix(body, []byte("\xef\xbb\xbf")))
	if len(trim) == 0 {
		return nil, fmt.Errorf("%w: empty body", ErrParse)
	}
	if trim[0] == '{' {
		return parseJSON(trim)
	}
	return parseXML(trim)
}

func newResponse() *Response {
	return &Response{Objects: map[string][]Instance{}, Values: map[string]string{}}
}

func parseXML(body []byte) (*Response, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }

	r := newResponse()
	seenRoot := false
	depth := 0
	// state for the element being read at depth 1 (object) / 2 (instance)
	var (
		objName   string
		objIsInst bool
		objText   strings.Builder
		inst      Instance
		paraName  *string
		leafName  string
		leafText  strings.Builder
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if seenRoot {
				break // tolerate trailing garbage / truncation
			}
			return nil, fmt.Errorf("%w: %v", ErrParse, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch depth {
			case 1:
				seenRoot = true
			case 2:
				objName, objIsInst = t.Name.Local, false
				objText.Reset()
			case 3:
				if t.Name.Local == "Instance" {
					objIsInst = true
					inst = Instance{}
					paraName = nil
				}
			case 4:
				leafName = t.Name.Local
				leafText.Reset()
			}
		case xml.CharData:
			switch depth {
			case 2:
				objText.Write(t)
			case 4:
				leafText.Write(t)
			}
		case xml.EndElement:
			switch depth {
			case 4:
				if inst != nil {
					switch leafName {
					case "ParaName":
						name := strings.TrimSpace(leafText.String())
						if paraName != nil { // previous name had no value
							inst[*paraName] = ""
						}
						paraName = &name
					case "ParaValue":
						if paraName != nil {
							inst[*paraName] = leafText.String()
							paraName = nil
						}
					default: // non-standard child: keep it as a field
						inst[leafName] = leafText.String()
					}
				}
			case 3:
				if inst != nil {
					if paraName != nil {
						inst[*paraName] = ""
					}
					r.Objects[objName] = append(r.Objects[objName], inst)
					inst, paraName = nil, nil
				}
			case 2:
				if objIsInst {
					if _, ok := r.Objects[objName]; !ok {
						r.Objects[objName] = nil
					}
				} else {
					v := strings.TrimSpace(objText.String())
					if objName == "encode" {
						for _, n := range strings.FieldsFunc(v, func(c rune) bool { return c == ',' || c == ';' || c == ' ' }) {
							r.Encoded = append(r.Encoded, n)
						}
					}
					if old, ok := r.Values[objName]; ok && old != "" {
						v = old + "," + v
					}
					r.Values[objName] = v
				}
			}
			depth--
		}
	}
	if !seenRoot {
		return nil, fmt.Errorf("%w: no root element", ErrParse)
	}
	return r, nil
}

func parseJSON(body []byte) (*Response, error) {
	var top map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParse, err)
	}
	r := newResponse()
	for k, v := range top {
		switch val := v.(type) {
		case map[string]any:
			if arr, ok := val["Instance"].([]any); ok {
				for _, it := range arr {
					if m, ok := it.(map[string]any); ok {
						r.Objects[k] = append(r.Objects[k], flatten(m))
					}
				}
				if enc, ok := val["encode"].(string); ok {
					r.Encoded = append(r.Encoded, strings.Split(enc, ",")...)
				}
				if _, ok := r.Objects[k]; !ok {
					r.Objects[k] = nil
				}
				continue
			}
			r.Objects[k] = []Instance{flatten(val)}
		case []any:
			for _, it := range val {
				if m, ok := it.(map[string]any); ok {
					r.Objects[k] = append(r.Objects[k], flatten(m))
				}
			}
		default:
			r.Values[k] = scalar(v)
		}
	}
	return r, nil
}

func flatten(m map[string]any) Instance {
	in := Instance{}
	for k, v := range m {
		switch v.(type) {
		case map[string]any, []any:
			b, _ := json.Marshal(v)
			in[k] = string(b)
		default:
			in[k] = scalar(v)
		}
	}
	return in
}

func scalar(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprint(x)
	}
}

// leadingFloat parses the leading number of s ("-21.3 dBm" -> -21.3).
func leadingFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= '0' && c <= '9') || c == '.' || ((c == '-' || c == '+') && end == 0) {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
