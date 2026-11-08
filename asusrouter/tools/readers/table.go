// Translation table readers (table.py).

package readers

import "github.com/eduardpaul/routerctl/asusrouter/tools/converters"

// TableEntry is one `(key, field, converter)` row of a translation table.
// Adapt typed converters with `converters.AsAny(converters.RawToInt)`.
type TableEntry[K ~string, F comparable] struct {
	Key       K
	Field     F
	Converter func(any) any
}

// ReadTable mirrors `read_table`: read a `(key, field, converter)` table
// into a field map.
//
// `key` resolves a table key to its raw data key (e.g. an indexed nvram
// template); when nil the key is used as-is. Fields whose raw value is
// absent or converts to no value are skipped.
func ReadTable[K ~string, F comparable](
	data map[string]any,
	table []TableEntry[K, F],
	key func(K) string,
) map[F]any {
	fields := map[F]any{}
	for _, entry := range table {
		rawKey := string(entry.Key)
		if key != nil {
			rawKey = key(entry.Key)
		}
		value := converters.RawConvert(data[rawKey], entry.Converter)
		if value != nil {
			fields[entry.Field] = value
		}
	}
	return fields
}
