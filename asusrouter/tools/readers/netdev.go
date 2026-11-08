// Reader for the ASUS netdev byte-counter payloads (netdev.py).

package readers

import (
	"regexp"
	"strconv"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Nested `'IFACE':{rx:0x..,tx:0x..}` from `update.cgi?output=netdev`
var netdevNestedRe = regexp.MustCompile(
	`'(` + converters.PyWordClass + `+)'` + ws + `*:` + ws + `*\{` + ws + `*rx` + ws + `*:` + ws + `*(0x[0-9a-fA-F]+)` + ws + `*,` +
		ws + `*tx` + ws + `*:` + ws + `*(0x[0-9a-fA-F]+)` + ws + `*\}`,
)

// Flat `"IFACE_rx":"0x.."` from the legacy appGet `netdev` hook
var netdevFlatRe = regexp.MustCompile(
	`"(` + converters.PyWordClass + `+)_(rx|tx)"` + ws + `*:` + ws + `*"(0x[0-9a-fA-F]+)"`,
)

// hex16 mirrors `int(value, 16)` for a `0x` literal. Values beyond 64
// bits (Python: big int) are reported as not parsed.
func hex16(value string) (int, bool) {
	v, err := strconv.ParseUint(value[2:], 16, 64)
	return int(v), err == nil
}

// ReadNetdev mirrors `read_netdev`: parse a netdev payload into
// per-interface byte counters.
//
// Handles both the `update.cgi` JS literal (nested, bare keys, single
// quotes) and the legacy appGet hook (flat `IFACE_rx` keys). Neither is
// parsed as JSON on purpose: both repeat `INTERNET` once per WAN unit on
// dual-WAN devices, so duplicates are summed per interface instead of
// being collapsed. A counter beyond 64 bits skips its entry.
func ReadNetdev(content string) map[string]map[string]int {
	result := map[string]map[string]int{}
	bucketOf := func(iface string) map[string]int {
		bucket, ok := result[iface]
		if !ok {
			bucket = map[string]int{}
			result[iface] = bucket
		}
		return bucket
	}
	for _, m := range netdevNestedRe.FindAllStringSubmatch(content, -1) {
		rx, okRx := hex16(m[2])
		tx, okTx := hex16(m[3])
		if !okRx || !okTx {
			continue
		}
		bucket := bucketOf(m[1])
		bucket["rx"] += rx
		bucket["tx"] += tx
	}
	for _, m := range netdevFlatRe.FindAllStringSubmatch(content, -1) {
		value, ok := hex16(m[3])
		if !ok {
			continue
		}
		bucketOf(m[1])[m[2]] += value
	}
	return result
}
