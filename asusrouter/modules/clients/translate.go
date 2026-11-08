package clients

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/units"
)

// Link rates come as binary Mibit/s; read them as bits/s
var readMibps = units.ReadDataRate(units.UnitOfDataRateMebibitPerSecond)

// Raw hook keys that are not clients
var skipKeys = map[string]bool{"maclist": true, "ClientAPILevel": true}

// `isWL` / `wireless` value -> frequency (0 = wired; 3 normalizes to 5G)
var wlFrequency = map[int]wifi.ARWiFiFrequency{
	1: wifi.ARWiFiFrequencyFreq2g,
	2: wifi.ARWiFiFrequencyFreq5g,
	4: wifi.ARWiFiFrequencyFreq6g,
}

// Asus band tag, e.g. `5G1` / `6G` / `2G2`; the index is 1-based and a
// bare tag (`5G`) means the first band (`5G1`)
// (`\n?` keeps Python's `$`, which also matches before a final newline)
var bandTagRe = regexp.MustCompile(`(?i)^(\d)G(\d*)\n?$`)

// Main fronthaul SDN type - a client there is not a guest
const sdnMain = "MAINFH"

// `isWL` value reported by some dualband firmwares for 5G
const iswlDualband = 3

// truthy mirrors Python truthiness of a parsed JSON value.
func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return true
}

// or mirrors Python `a or b`.
func or(a, b any) any {
	if truthy(a) {
		return a
	}
	return b
}

// clean mirrors `_clean`: strip the firmware's trailing-space keys (e.g.
// `conn_ts `).
func clean(entry map[string]any) map[string]any {
	out := make(map[string]any, len(entry))
	for key, value := range entry {
		k := key
		if s := converters.RawToStr(key); s != nil && *s != "" {
			k = *s
		}
		out[k] = value
	}
	return out
}

// rate mirrors `_rate`: convert a Mibit/s link rate to bits/s, or nil when
// absent.
func rate(value any) *float64 {
	if converters.RawToStr(value) == nil {
		return nil
	}
	r := readMibps(value)
	return &r
}

// sinceTs mirrors `_since_ts`: convert a unix connection timestamp to a
// datetime.
func sinceTs(value any) *time.Time {
	ts := converters.RawToInt(value)
	if ts == nil || *ts == 0 {
		return nil
	}
	t := time.Unix(int64(*ts), 0).UTC()
	return &t
}

// pyInt mirrors Python `int(str)` for the uptime parts.
func pyInt(s string) (int, bool) {
	n, err := strconv.Atoi(strings.ReplaceAll(converters.PyStrip(s), "_", ""))
	return n, err == nil
}

// uptimeTimedelta mirrors `_uptime_timedelta`: parse a `HH:MM:SS` uptime
// string into a duration.
func uptimeTimedelta(value string) time.Duration {
	part := strings.Split(value, ":")
	if len(part) < 3 {
		return 0
	}
	h, okH := pyInt(part[len(part)-3])
	m, okM := pyInt(part[len(part)-2])
	s, okS := pyInt(part[len(part)-1])
	if !okH || !okM || !okS {
		return 0
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
}

// now is the clock of `_since_delta` (a seam for tests).
var now = time.Now

// sinceDelta mirrors `_since_delta`: convert a `HH:MM:SS` connection
// duration to a datetime.
func sinceDelta(value any) *time.Time {
	text := converters.RawToStr(value)
	if text == nil || *text == "" {
		return nil
	}

	current := now().UTC().Truncate(time.Second)
	t := current.Add(-uptimeTimedelta(*text))
	return &t
}

// bandFrequency mirrors `_band_frequency`: map a specific band to its
// frequency.
func bandFrequency(band wifi.ARWiFiBand) wifi.ARWiFiFrequency {
	value := string(band)
	if len(value) > 2 {
		value = value[:2]
	}
	return wifi.ARWiFiFrequencyFromValue(value)
}

// tagToBand mirrors `_tag_to_band`: resolve an Asus band tag (`5G1`, `6G`,
// ...) to a band, or nil.
func tagToBand(tag string) *wifi.ARWiFiBand {
	match := bandTagRe.FindStringSubmatch(tag)
	if match == nil {
		return nil
	}
	freq, index := match[1], match[2]
	if index == "" {
		index = "1"
	}
	band := wifi.ARWiFiBandFromValue(freq + "g" + index)
	if band == wifi.ARWiFiBandUnknown {
		return nil
	}
	return &band
}

// isMeshNode mirrors `_is_mesh_node`: whether the entry is an AiMesh node
// rather than a client.
func isMeshNode(entry map[string]any) bool {
	b := converters.RawToBool(entry["amesh_isRe"])
	return b != nil && *b
}

// name mirrors `_name`: pick a display name: nickName, then name, across
// entries.
func name(entries ...map[string]any) *string {
	for _, key := range []string{"nickName", "name"} {
		for _, entry := range entries {
			if value := converters.RawToStr(entry[key]); value != nil && *value != "" {
				return value
			}
		}
	}
	return nil
}

// sortedKeys returns the keys of a parsed object, sorted. Parsed JSON
// objects do not keep the wire order Python iterates in, so links built
// from one are ordered by key instead.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// mloLiveLinks mirrors `_mlo_live_links`: build MLO links from the live
// per-link metrics.
func mloLiveLinks(mloLinks map[string]any) []ARClientLink {
	links := []ARClientLink{}
	for _, radio := range sortedKeys(mloLinks) {
		info, ok := mloLinks[radio].(map[string]any)
		if !ok {
			continue
		}
		band := tagToBand(radio)
		link := NewARClientLink()
		if band != nil {
			link.Frequency = bandFrequency(*band)
		}
		link.Band = band
		link.Mac = identifiers.MacAddressFromValueSafe(info["mac"])
		link.Rssi = converters.RawToInt(info["rssi"])
		link.RxSpeed = rate(info["rx"])
		link.TxSpeed = rate(info["tx"])
		link.ConnectedSince = sinceDelta(info["conn_time"])
		links = append(links, link)
	}
	return links
}

// mloDbLinks mirrors `_mlo_db_links`: build MLO links from the database
// per-band MAC fields.
func mloDbLinks(db map[string]any) []ARClientLink {
	links := []ARClientLink{}
	for _, field := range sortedKeys(db) {
		if field == "mlo_all_mac" || !(strings.HasPrefix(field, "mlo_") && strings.HasSuffix(field, "_mac")) {
			continue
		}
		// `field[len("mlo_"):-len("_mac")]` (empty when the affixes overlap)
		tag := ""
		if len(field) > len("mlo_")+len("_mac") {
			tag = field[len("mlo_") : len(field)-len("_mac")]
		}
		band := tagToBand(tag)
		mac := identifiers.MacAddressFromValueSafe(db[field])
		if band != nil && mac != nil {
			link := NewARClientLink()
			link.Frequency = bandFrequency(*band)
			link.Band = band
			link.Mac = mac
			links = append(links, link)
		}
	}
	return links
}

// singleLink mirrors `_single_link`: build the single wireless link, of
// which only frequency is known.
func singleLink(live, db map[string]any) []ARClientLink {
	unit := converters.RawToInt(or(live["isWL"], db["wireless"]))
	if unit != nil && *unit == iswlDualband {
		two := 2
		unit = &two
	}
	if unit == nil {
		return []ARClientLink{}
	}
	frequency, ok := wlFrequency[*unit]
	if !ok {
		return []ARClientLink{}
	}
	link := NewARClientLink()
	link.Frequency = frequency
	link.Rssi = converters.RawToInt(live["rssi"])
	link.RxSpeed = rate(live["curRx"])
	link.TxSpeed = rate(live["curTx"])
	return []ARClientLink{link}
}

// buildLinks mirrors `_build_links`: build wireless links, modelling MLO
// as one link per radio.
func buildLinks(live, db map[string]any) (bool, []ARClientLink) {
	isTrue := func(v any) bool { b := converters.RawToBool(v); return b != nil && *b }
	mlo := isTrue(live["mlo"]) || isTrue(db["mlo"])
	mloLinks, isMap := live["mlo_links"].(map[string]any)

	if mlo && isMap && len(mloLinks) > 0 {
		return true, mloLiveLinks(mloLinks)
	}
	if mlo {
		if links := mloDbLinks(db); len(links) > 0 {
			return true, links
		}
	}
	return false, singleLink(live, db)
}

// buildConnection mirrors `_build_connection`: build the current
// connection from the live (and db) entry.
func buildConnection(live, db map[string]any, router *identifiers.MacAddress) ARClientConnection {
	wiredUnit := converters.RawToInt(or(live["isWL"], db["wireless"]))
	wired := wiredUnit != nil && *wiredUnit == 0
	mlo, links := false, []ARClientLink{}
	if !wired {
		mlo, links = buildLinks(live, db)
	}

	sdnType := converters.RawToStr(live["sdn_type"])
	isGN := converters.RawToStr(live["isGN"])
	guest := isGN != nil && *isGN != "" && (sdnType == nil || *sdnType != sdnMain)

	// wlAuth / ipMethod arrive upper/mixed case; the enums are lowercase
	auth := converters.RawToStr(live["wlAuth"])
	ipMethod := converters.RawToStr(live["ipMethod"])

	connection := NewARClientConnection()
	connection.Type = common.ARConnectionTypeWireless
	if wired {
		connection.Type = common.ARConnectionTypeWired
	}
	connection.Ip = identifiers.IpAddressFromValueSafe(live["ip"])
	if ipMethod != nil && *ipMethod != "" {
		connection.IpMethod = common.ARIPMethodFromValue(strings.ToLower(*ipMethod))
	} else {
		connection.IpMethod = common.ARIPMethodFromValue(nil)
	}
	connection.InternetAccess = converters.RawToBool(live["internetState"])
	connection.Node = identifiers.MacAddressFromValueSafe(live["amesh_papMac"])
	if connection.Node == nil {
		connection.Node = router
	}
	connection.Ssid = converters.RawToStr(live["ssid"])
	if auth != nil && *auth != "" {
		security := wifi.ARWiFiAuthFromValue(strings.ToLower(*auth))
		connection.Security = &security
	}
	connection.Guest = guest
	connection.Sdn = sdnType
	connection.Mlo = mlo
	connection.ConnectedSince = sinceTs(db["conn_ts"])
	connection.Links = links
	return connection
}

// buildClient mirrors `_build_client`: build a single client from its live
// and database entries.
func buildClient(mac identifiers.MacAddress, live, db map[string]any, router *identifiers.MacAddress) ARClient {
	isOnline := converters.RawToBool(live["isOnline"])
	online := isOnline != nil && *isOnline

	client := NewARClient(mac)
	client.Online = online
	client.Name = name(live, db)
	client.Vendor = converters.RawToStr(live["vendor"])
	if client.Vendor == nil || *client.Vendor == "" {
		client.Vendor = converters.RawToStr(db["vendor"])
	}
	client.VendorClass = converters.RawToStr(db["vendorclass"])
	client.DeviceType = common.ARDeviceTypeFromValue(or(live["type"], db["type"]))
	client.OsType = converters.RawToInt(db["os_type"])
	client.BoundNode = identifiers.MacAddressFromValueSafe(or(live["amesh_bind_mac"], db["amesh_bind_mac"]))
	client.InternetMode = common.ARInternetModeFromValue(live["internetMode"])
	if online {
		connection := buildConnection(live, db, router)
		client.Connection = &connection
	}
	return client
}

// macEntry is one MAC-keyed entry of `_entries`, in the order read.
type macEntry struct {
	mac   identifiers.MacAddress
	entry map[string]any
}

// entries mirrors `_entries`: extract the per-MAC entries of one hook,
// MAC-keyed and cleaned (a later raw key resolving to the same MAC
// replaces the earlier one in place, like a dict).
func entries(raw map[string]any, key string) ([]macEntry, map[identifiers.MacAddress]map[string]any) {
	var order []macEntry
	byMac := map[identifiers.MacAddress]map[string]any{}
	section, ok := raw[key].(map[string]any)
	if !ok {
		return order, byMac
	}
	for _, macRaw := range sortedKeys(section) {
		entry, isMap := section[macRaw].(map[string]any)
		if skipKeys[macRaw] || !isMap {
			continue
		}
		mac := identifiers.MacAddressFromValueSafe(macRaw)
		if mac == nil {
			continue
		}
		cleaned := clean(entry)
		if _, seen := byMac[*mac]; !seen {
			order = append(order, macEntry{*mac, cleaned})
		} else {
			for i := range order {
				if order[i].mac == *mac {
					order[i].entry = cleaned
				}
			}
		}
		byMac[*mac] = cleaned
	}
	return order, byMac
}

// BuildClients mirrors `build_clients`: build all clients from the two
// hook payloads, merged.
//
// `get_clientlist` is the primary (live) source; the database adds
// identity-only clients not present there. AiMesh nodes and the router
// itself are excluded.
func BuildClients(raw any, identity *device.ARDeviceIdentity) map[identifiers.MacAddress]ARClient {
	data, ok := raw.(map[string]any)
	if !ok {
		return map[identifiers.MacAddress]ARClient{}
	}

	var router *identifiers.MacAddress
	if identity != nil {
		router = identity.Mac()
	}
	isRouter := func(mac identifiers.MacAddress) bool { return router != nil && mac == *router }
	live, _ := entries(data, "get_clientlist")
	db, dbByMac := entries(data, "get_clientlist_from_json_database")

	clients := map[identifiers.MacAddress]ARClient{}
	for _, e := range live {
		if isMeshNode(e.entry) || isRouter(e.mac) {
			continue
		}
		dbEntry := dbByMac[e.mac]
		if dbEntry == nil {
			dbEntry = map[string]any{}
		}
		clients[e.mac] = buildClient(e.mac, e.entry, dbEntry, router)
	}
	for _, e := range db {
		if _, seen := clients[e.mac]; seen || isMeshNode(e.entry) || isRouter(e.mac) {
			continue
		}
		clients[e.mac] = buildClient(e.mac, map[string]any{}, e.entry, router)
	}
	return clients
}
