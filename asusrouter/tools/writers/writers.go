// Package writers ports `asusrouter/tools/writers`.
package writers

import (
	"sort"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// RequestDelimiter mirrors `REQUEST_DELIMITER`.
var RequestDelimiter = map[consts.RequestType]string{
	consts.RequestTypeGet:  "&",
	consts.RequestTypePost: ";",
}

// KeyValue is one entry of an ordered mapping (Python dicts keep their
// insertion order, Go maps do not).
type KeyValue struct {
	Key   string
	Value any
}

// valueToStr mirrors `_value_to_str`: render a request value as a
// device-friendly string.
func valueToStr(value any) string {
	switch v := value.(type) {
	case identifiers.MacAddress:
		return v.AsAsus()
	case *identifiers.MacAddress:
		if v != nil {
			return v.AsAsus()
		}
		return ""
	case bool:
		if v {
			return "1"
		}
		return "0"
	case nil:
		return ""
	}
	// Python `str()` (redacting types stay redacted)
	return security.PyStrOf(value)
}

// DictToRequest mirrors `dict_to_request`: convert an ordered mapping to
// a request string.
//
//   - For RequestTypeGet keys and values are URL-quoted (quote_plus) and
//     pairs are joined with '&'.
//   - For RequestTypePost keys and values are escaped for single-quotes
//     and pairs are joined with ';'.
func DictToRequest(data []KeyValue, requestType consts.RequestType) string {
	if len(data) == 0 {
		return ""
	}

	isGet := requestType == consts.RequestTypeGet
	parts := make([]string, 0, len(data))

	for _, item := range data {
		valStr := valueToStr(item.Value)
		if isGet {
			parts = append(parts, QuotePlus(item.Key)+"="+QuotePlus(valStr))
		} else {
			// Escape single quotes to avoid breaking the 'key':'value' syntax
			escK := strings.ReplaceAll(item.Key, "'", "\\'")
			escV := strings.ReplaceAll(valStr, "'", "\\'")
			parts = append(parts, "'"+escK+"':'"+escV+"'")
		}
	}

	return strings.Join(parts, RequestDelimiter[requestType])
}

// MapToRequest is DictToRequest for a Go map, with the keys sorted.
func MapToRequest(data map[string]any, requestType consts.RequestType) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]KeyValue, len(keys))
	for i, key := range keys {
		items[i] = KeyValue{Key: key, Value: data[key]}
	}
	return DictToRequest(items, requestType)
}

// QuotePlus mirrors Python `urllib.parse.quote_plus` (safe=""): letters,
// digits and `_.-~` are kept, spaces become '+', every other UTF-8 byte
// becomes %XX.
func QuotePlus(text string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '_', c == '.', c == '-', c == '~':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0x0f])
		}
	}
	return b.String()
}
