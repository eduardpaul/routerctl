// Package endpoint mirrors asusrouter/modules/endpoint/__init__.py.
package endpoint

import (
	"encoding/json"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// AREndpointMeta mirrors `AREndpointMeta`.
type AREndpointMeta struct {
	RequestType consts.RequestType
	// Minimum log security level at which the request payload may be
	// logged raw; below it the payload is redacted
	PayloadSensitivity security.ARSecurityLevel
	RawPayload         bool
}

var (
	defaultMeta = AREndpointMeta{RequestType: consts.RequestTypePost, PayloadSensitivity: security.ARSecurityLevelDefault}
	getMeta     = AREndpointMeta{RequestType: consts.RequestTypeGet, PayloadSensitivity: security.ARSecurityLevelDefault}
	rawPostMeta = AREndpointMeta{RequestType: consts.RequestTypePost, PayloadSensitivity: security.ARSecurityLevelDefault, RawPayload: true}
	// Raw POST whose body carries user data (e.g. real IPs) - never log by default
	rawPostSensitiveMeta = AREndpointMeta{RequestType: consts.RequestTypePost, PayloadSensitivity: security.ARSecurityLevelUnsafe, RawPayload: true}
)

var endpointMeta = map[AREndpoint]AREndpointMeta{
	AREndpointChpass:                       rawPostSensitiveMeta,
	AREndpointDdnsClean:                    getMeta,
	AREndpointDdnsUnregister:               getMeta,
	AREndpointFetchDiagnosticsActiveClient: getMeta,
	AREndpointFetchDiagnosticsData:         getMeta,
	AREndpointFetchNetwork:                 getMeta,
	AREndpointFetchPortStatus:              getMeta,
	AREndpointFetchTrafficBackhaul:         getMeta,
	AREndpointFetchTrafficEthernet:         getMeta,
	AREndpointFetchTrafficWifi:             getMeta,
	AREndpointFetchUpdate:                  getMeta,
	AREndpointFetchVpnOpenvpnStatus:        getMeta,
	AREndpointFetchVpnStatus:               getMeta,
	AREndpointRunPing:                      getMeta,
	AREndpointRunSpeedtest:                 rawPostMeta,
	AREndpointSetAura:                      getMeta,
	AREndpointSetSpeedtestStartTime:        rawPostMeta,
	// Might contain sensitive data, including passwords
	AREndpointStartApply:            rawPostSensitiveMeta,
	AREndpointWriteSpeedtestHistory: rawPostSensitiveMeta,
	AREndpointLogin:                 {RequestType: consts.RequestTypePost, PayloadSensitivity: security.ARSecurityLevelUnsafe},
}

// GetEndpointMeta mirrors `get_endpoint_meta`.
func GetEndpointMeta(e AREndpoint) AREndpointMeta {
	if m, ok := endpointMeta[e]; ok {
		return m
	}
	return defaultMeta
}

// GetEndpointRequestType mirrors `get_endpoint_request_type`.
func GetEndpointRequestType(e AREndpoint) consts.RequestType {
	return GetEndpointMeta(e).RequestType
}

// GetEndpointPayloadSensitivity mirrors `get_endpoint_payload_sensitivity`.
func GetEndpointPayloadSensitivity(e AREndpoint) security.ARSecurityLevel {
	return GetEndpointMeta(e).PayloadSensitivity
}

// GetEndpointRawPayload mirrors `get_endpoint_raw_payload`.
func GetEndpointRawPayload(e AREndpoint) bool {
	return GetEndpointMeta(e).RawPayload
}

// BuildPushRequest mirrors `build_push_request`: a compact JSON object
// with an `action_mode` and optional command fields. Keys keep insertion
// order like Python dicts: action_mode first, then payload in the given
// order.
func BuildPushRequest(actionMode string, payload [][2]any) string {
	if actionMode == "" {
		actionMode = string(common.ARActionModeApply)
	}
	return orderedJSON(append([][2]any{{common.ActionModeKey, actionMode}}, payload...))
}

// orderedJSON renders key/value pairs as a compact JSON object, keeping
// order and letting later keys override earlier ones in place (dict.update).
func orderedJSON(pairs [][2]any) string {
	index := map[string]int{}
	var keys []string
	values := map[string]any{}
	for _, kv := range pairs {
		k := converters.RawToStr(kv[0])
		if k == nil {
			continue
		}
		if _, ok := index[*k]; !ok {
			index[*k] = len(keys)
			keys = append(keys, *k)
		}
		values[*k] = kv[1]
	}
	out := []byte{'{'}
	for i, k := range keys {
		if i > 0 {
			out = append(out, ',')
		}
		kb, _ := json.Marshal(k)
		vb, err := marshalCompact(values[k])
		if err != nil {
			vb = []byte("null")
		}
		out = append(out, kb...)
		out = append(out, ':')
		out = append(out, vb...)
	}
	return string(append(out, '}'))
}

func marshalCompact(v any) ([]byte, error) {
	// Python json.dumps escapes non-ASCII and does not escape <>&
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return pyJSONEscape(b), nil
}

// pyJSONEscape undoes Go's HTML-safe escaping and applies Python's default
// ensure_ascii=True escaping, so bodies are byte-identical to Python.
func pyJSONEscape(b []byte) []byte {
	s := string(b)
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+5 < len(s) && s[i+1] == 'u' {
			switch s[i+2 : i+6] {
			case "003c":
				out = append(out, '<')
				i += 5
				continue
			case "003e":
				out = append(out, '>')
				i += 5
				continue
			case "0026":
				out = append(out, '&')
				i += 5
				continue
			}
		}
		out = append(out, s[i])
	}
	return asciiEscape(out)
}

func asciiEscape(b []byte) []byte {
	var out []byte
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			out = append(out, byte(r))
		case r <= 0xFFFF:
			out = append(out, []byte(jsonU(r))...)
		default:
			r -= 0x10000
			out = append(out, []byte(jsonU(0xD800+(r>>10)))...)
			out = append(out, []byte(jsonU(0xDC00+(r&0x3FF)))...)
		}
	}
	return out
}

func jsonU(r rune) string {
	const hex = "0123456789abcdef"
	return string([]byte{'\\', 'u', hex[(r>>12)&0xF], hex[(r>>8)&0xF], hex[(r>>4)&0xF], hex[r&0xF]})
}

// Reader parses the raw content of an endpoint.
type Reader func(content string) any

var endpointReader = map[AREndpoint]Reader{
	AREndpointFetchFirmwareUpdate:   func(c string) any { return readers.ReadJSVariables(c) },
	AREndpointFetchOnboarding:       func(c string) any { return readers.ReadJSVariables(c) },
	AREndpointFetchSysinfo:          func(c string) any { return readers.ReadJSVariables(c) },
	AREndpointFetchTemperature:      func(c string) any { return readers.ReadJSVariables(c) },
	AREndpointFetchPortsEthernet:    func(c string) any { return ReadWanLanStatus(c) },
	AREndpointFetchUpdate:           func(c string) any { return readers.ReadNetdev(c) },
	AREndpointFetchVpnOpenvpnStatus: func(c string) any { return derefStr(converters.RawToStr(c)) },
	AREndpointFetchVpnStatus:        func(c string) any { return readers.ReadJSVariables(c) },
}

func derefStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// GetEndpointReader mirrors `get_endpoint_reader`.
func GetEndpointReader(e AREndpoint) Reader {
	if r, ok := endpointReader[e]; ok {
		return r
	}
	return func(c string) any { return readers.ReadJSONContent(c) }
}
