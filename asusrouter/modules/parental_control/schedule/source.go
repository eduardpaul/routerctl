// Package schedule mirrors asusrouter/modules/parental_control/schedule/.
package schedule

import (
	"context"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	pc "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// Rules-engine master toggle
const KeyState = nvram.ARNvramTypeParentalControlState

// Parallel per-rule lists, aligned by position and `>`-joined
const (
	KeyMac     = nvram.ARNvramTypeParentalControlMac
	KeyName    = nvram.ARNvramTypeParentalControlName
	KeyType    = nvram.ARNvramTypeParentalControlType
	KeyTimemap = nvram.ARNvramTypeParentalControlTimemap
)

// PcRequest is the full NVRAM request for the parental control state,
// built once.
var PcRequest = []nvram.ARNvramType{
	KeyState,
	KeyMac,
	KeyName,
	KeyType,
	KeyTimemap,
}

// ARParentalControlRule is a rule: it shares the shape the read source
// emits, for a clean round-trip (Python `action.py`).
type ARParentalControlRule = map[pc.ARParentalControlField]any

// ARParentalControlSource mirrors `ARParentalControlSource`: the
// AsusRouter parental control data source.
type ARParentalControlSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARParentalControlSource) SourceName() string { return "ARParentalControlSource" }

// ARParentalControlSourceUniversal is the universal instance - preferred.
var ARParentalControlSourceUniversal = &ARParentalControlSource{}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch the parental control configuration through the NVRAM module
	return nvramsource.AsyncFetchValues(ctx, c, PcRequest, nil)
}

func splitList(raw any) []string {
	// Decode a `>`-joined parallel list into its positional fields
	text := readers.Decode(raw)
	if text == "" {
		return []string{}
	}
	return strings.Split(text, ">")
}

// RuleScheduleMode mirrors `rule_schedule_mode`: a rule's schedule mode,
// the explicit MODE field else the timemap.
func RuleScheduleMode(rule ARParentalControlRule) pc.ARParentalControlScheduleMode {
	if mode := rule[pc.ARParentalControlFieldMode]; mode != nil {
		return pc.ARParentalControlScheduleModeFromValue(mode)
	}
	return TimemapMode(orString(rule[pc.ARParentalControlFieldTimemap]))
}

// orString mirrors `str(value or "")`.
func orString(value any) string {
	if !truthy(value) {
		return ""
	}
	return converters.PyStr(value)
}

// scheduleEntries reads a SCHEDULE field value as entries (nil when the
// field is absent).
func scheduleEntries(value any) ([]ARScheduleEntry, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case []ARScheduleEntry:
		return v, true
	case []any:
		out := make([]ARScheduleEntry, 0, len(v))
		for _, item := range v {
			switch e := item.(type) {
			case ARScheduleEntry:
				out = append(out, e)
			case *ARScheduleEntry:
				if e != nil {
					out = append(out, *e)
				}
			}
		}
		return out, true
	}
	return []ARScheduleEntry{}, true
}

// RuleEntryCount mirrors `rule_entry_count`: count the schedule windows a
// rule serializes to.
//
// Mirrors `ruleTimemap`: a parsed `SCHEDULE` wins, else the raw
// `TIMEMAP`, else the seeded default.
func RuleEntryCount(rule ARParentalControlRule) int {
	if schedule, ok := scheduleEntries(rule[pc.ARParentalControlFieldSchedule]); ok {
		return len(schedule)
	}
	raw := rule[pc.ARParentalControlFieldTimemap]
	if !truthy(raw) {
		raw = DefaultTimemap
	}
	return len(ParseTimemap(raw))
}

func parseRule(mac, name, type_, timemap string) ARParentalControlRule {
	// Build a single rule dict from its aligned list fields
	var macValue any = mac
	if parsed := identifiers.MacAddressFromValueSafe(mac); parsed != nil {
		macValue = *parsed
	}
	timemapStr := ""
	if s := converters.RawToStr(timemap); s != nil {
		timemapStr = *s
	}
	nameStr := ""
	if s := converters.RawToStr(name); s != nil {
		nameStr = *s
	}

	return ARParentalControlRule{
		pc.ARParentalControlFieldMac:      macValue,
		pc.ARParentalControlFieldName:     nameStr,
		pc.ARParentalControlFieldType:     pc.ARParentalControlTypeFromValue(type_),
		pc.ARParentalControlFieldMode:     TimemapMode(timemapStr),
		pc.ARParentalControlFieldSchedule: ParseTimemap(timemapStr),
		pc.ARParentalControlFieldTimemap:  timemapStr,
	}
}

func parseRules(data map[source.ID]any) []ARParentalControlRule {
	// Parse the parallel per-rule lists into rule dicts

	// The four lists are positional; pad short ones so rows stay aligned
	columns := [][]string{
		splitList(data[source.IDOf(KeyMac)]),
		splitList(data[source.IDOf(KeyName)]),
		splitList(data[source.IDOf(KeyType)]),
		splitList(data[source.IDOf(KeyTimemap)]),
	}
	rows := 0
	for _, column := range columns {
		if len(column) > rows {
			rows = len(column)
		}
	}
	at := func(column []string, i int) string {
		if i < len(column) {
			return column[i]
		}
		return ""
	}

	out := []ARParentalControlRule{}
	for i := 0; i < rows; i++ {
		mac := at(columns[0], i)
		if mac == "" {
			continue
		}
		out = append(out, parseRule(mac, at(columns[1], i), at(columns[2], i), at(columns[3], i)))
	}
	return out
}

func ruleTimemap(rule ARParentalControlRule) string {
	// Serialize a rule's timemap from its schedule entries, else the raw
	// one.
	//
	// A parsed `SCHEDULE` is authoritative when present; otherwise the raw
	// `TIMEMAP` string passes through. Either way the rule's mode is
	// applied.
	mode := RuleScheduleMode(rule)

	if schedule, ok := scheduleEntries(rule[pc.ARParentalControlFieldSchedule]); ok {
		return SerializeTimemap(schedule, mode)
	}

	raw := DefaultTimemap
	if value := rule[pc.ARParentalControlFieldTimemap]; truthy(value) {
		raw = converters.PyStr(value)
	}
	return ApplyMode(raw, mode)
}

func ruleField(rule ARParentalControlRule) [4]string {
	// Serialize a rule dict into its (mac, name, type, timemap) fields
	var macStr string
	switch mac := rule[pc.ARParentalControlFieldMac].(type) {
	case identifiers.MacAddress:
		macStr = mac.AsAsus()
	case *identifiers.MacAddress:
		if mac != nil {
			macStr = mac.AsAsus()
		}
	default:
		macStr = orString(mac)
	}

	type_ := pc.ARParentalControlTypeFromValue(rule[pc.ARParentalControlFieldType])

	return [4]string{
		macStr,
		orString(rule[pc.ARParentalControlFieldName]),
		converters.PyStr(int(type_)),
		ruleTimemap(rule),
	}
}

// SerializeRules mirrors `serialize_rules`: rule dicts into the parallel
// `>`-joined nvram lists, in Python order.
func SerializeRules(rules []ARParentalControlRule) [][2]any {
	columns := make([][4]string, len(rules))
	for i, rule := range rules {
		columns[i] = ruleField(rule)
	}
	join := func(index int) string {
		parts := make([]string, len(columns))
		for i, column := range columns {
			parts[i] = column[index]
		}
		return strings.Join(parts, ">")
	}
	return [][2]any{
		{string(KeyMac), join(0)},
		{string(KeyName), join(1)},
		{string(KeyType), join(2)},
		{string(KeyTimemap), join(3)},
	}
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate raw parental control nvram into a structured dict
	data, ok := raw.(map[source.ID]any)
	if !ok || len(data) == 0 {
		return map[pc.ARParentalControlField]any{}
	}

	state := converters.RawToBool(data[source.IDOf(KeyState)])
	return map[pc.ARParentalControlField]any{
		pc.ARParentalControlFieldState: state != nil && *state,
		pc.ARParentalControlFieldRules: parseRules(data),
	}
}

// truthy mirrors Python truthiness for rule field values.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case int:
		return t != 0
	case []any:
		return len(t) > 0
	}
	return true
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARParentalControlSource",
		Default:   func() source.DataSource { return &ARParentalControlSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
