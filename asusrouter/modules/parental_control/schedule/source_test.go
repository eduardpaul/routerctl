package schedule

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	pc "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

type F = pc.ARParentalControlField

const (
	fMac      = pc.ARParentalControlFieldMac
	fName     = pc.ARParentalControlFieldName
	fType     = pc.ARParentalControlFieldType
	fMode     = pc.ARParentalControlFieldMode
	fSchedule = pc.ARParentalControlFieldSchedule
	fTimemap  = pc.ARParentalControlFieldTimemap
	fState    = pc.ARParentalControlFieldState
	fRules    = pc.ARParentalControlFieldRules

	modeOffline = pc.ARParentalControlScheduleModeOffline
	modeOnline  = pc.ARParentalControlScheduleModeOnline
	modeUnknown = pc.ARParentalControlScheduleModeUnknown
)

// Two aligned rules across the parallel `>`-joined lists
const (
	rawMac     = "AA:BB:CC:DD:EE:FF&#6211:22:33:44:55:66"
	rawName    = "phone&#62tablet"
	rawType    = "2&#621"
	rawTimemap = "W03E21000700&#60W04122000800&#62W01E21000700"
)

func nvramData(values map[nvram.ARNvramType]any) map[source.ID]any {
	out := map[source.ID]any{}
	for k, v := range values {
		out[source.IDOf(k)] = v
	}
	return out
}

func translated(values map[nvram.ARNvramType]any) map[F]any {
	return translateState(nvramData(values), nil).(map[F]any)
}

func rulesOf(t *testing.T, values map[nvram.ARNvramType]any) []ARParentalControlRule {
	t.Helper()
	return translated(values)[fRules].([]ARParentalControlRule)
}

func serialized(rules ...ARParentalControlRule) map[string]any {
	out := map[string]any{}
	for _, kv := range SerializeRules(rules) {
		out[kv[0].(string)] = kv[1]
	}
	return out
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()
	var requested []source.DataSource
	c := &registry.Context{FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
		requested = srcs
		return nvramData(map[nvram.ARNvramType]any{KeyState: "1"}), nil
	}}
	got, _ := fetchState(ctx, c, ARParentalControlSourceUniversal)
	want := []source.DataSource{KeyState, KeyMac, KeyName, KeyType, KeyTimemap}
	if len(got.(map[source.ID]any)) != 1 || !reflect.DeepEqual(requested, want) {
		t.Errorf("fetches_via_nvram: %v %v", got, requested)
	}
	if got, _ := fetchState(ctx, &registry.Context{}, ARParentalControlSourceUniversal); len(got.(map[source.ID]any)) != 0 {
		t.Errorf("no_get_data_callback: %v", got)
	}
}

func TestTranslateState(t *testing.T) {
	for _, data := range []any{nil, map[source.ID]any{}, "text", []any{}} {
		if got := translateState(data, nil).(map[F]any); len(got) != 0 {
			t.Errorf("empty %v: %v", data, got)
		}
	}

	for state, want := range map[any]bool{"1": true, "0": false, nil: false} {
		got := translated(map[nvram.ARNvramType]any{"MULTIFILTER_ALL": state})
		if got[fState] != want || len(got[fRules].([]ARParentalControlRule)) != 0 {
			t.Errorf("state %v: %v", state, got)
		}
	}

	rules := rulesOf(t, map[nvram.ARNvramType]any{
		"MULTIFILTER_ALL":                  "1",
		"MULTIFILTER_MAC":                  rawMac,
		"MULTIFILTER_DEVICENAME":           rawName,
		"MULTIFILTER_ENABLE":               rawType,
		"MULTIFILTER_MACFILTER_DAYTIME_V2": rawTimemap,
	})
	if len(rules) != 2 {
		t.Fatalf("rules_aligned: %v", rules)
	}
	first, second := rules[0], rules[1]
	if m, ok := first[fMac].(identifiers.MacAddress); !ok || m.AsAsus() != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("first mac %v", first[fMac])
	}
	if first[fName] != "phone" || first[fType] != pc.ARParentalControlTypeBlock || first[fMode] != modeOffline || first[fTimemap] != "W03E21000700<W04122000800" {
		t.Errorf("first %v", first)
	}
	if second[fName] != "tablet" || second[fType] != pc.ARParentalControlTypeTime || second[fMode] != modeOffline || second[fTimemap] != "W01E21000700" {
		t.Errorf("second %v", second)
	}

	rules = rulesOf(t, map[nvram.ARNvramType]any{
		"MULTIFILTER_MAC":        "&#62AA:BB:CC:DD:EE:FF",
		"MULTIFILTER_DEVICENAME": "&#62kept",
		"MULTIFILTER_ENABLE":     "2&#622",
	})
	if len(rules) != 1 || rules[0][fName] != "kept" {
		t.Errorf("empty_mac_row_skipped: %v", rules)
	}

	rules = rulesOf(t, map[nvram.ARNvramType]any{"MULTIFILTER_MAC": "not-a-mac"})
	if len(rules) != 1 || rules[0][fMac] != "not-a-mac" {
		t.Errorf("unparseable_mac_kept_raw: %v", rules)
	}

	rules = rulesOf(t, map[nvram.ARNvramType]any{"MULTIFILTER_MAC": "AA:BB:CC:DD:EE:FF"})
	if len(rules) != 1 || rules[0][fName] != "" || rules[0][fType] != pc.ARParentalControlTypeUnknown || rules[0][fMode] != modeUnknown || rules[0][fTimemap] != "" {
		t.Errorf("ragged_lists_padded: %v", rules)
	}
}

func TestSerializeRules(t *testing.T) {
	empty := map[string]any{"MULTIFILTER_MAC": "", "MULTIFILTER_DEVICENAME": "", "MULTIFILTER_ENABLE": "", "MULTIFILTER_MACFILTER_DAYTIME_V2": ""}
	if got := serialized(); !reflect.DeepEqual(got, empty) {
		t.Errorf("empty: %v", got)
	}
	// Order matches Python
	order := SerializeRules(nil)
	if order[0][0] != "MULTIFILTER_MAC" || order[1][0] != "MULTIFILTER_DEVICENAME" || order[2][0] != "MULTIFILTER_ENABLE" || order[3][0] != "MULTIFILTER_MACFILTER_DAYTIME_V2" {
		t.Errorf("order: %v", order)
	}

	mac := *identifiers.MacAddressFromValueSafe("aa:bb:cc:dd:ee:ff")
	got := serialized(ARParentalControlRule{fMac: mac, fName: "pc", fType: pc.ARParentalControlTypeBlock, fTimemap: "W01E21000700"})
	if want := (map[string]any{"MULTIFILTER_MAC": "AA:BB:CC:DD:EE:FF", "MULTIFILTER_DEVICENAME": "pc", "MULTIFILTER_ENABLE": "2", "MULTIFILTER_MACFILTER_DAYTIME_V2": "W01E21000700"}); !reflect.DeepEqual(got, want) {
		t.Errorf("mac_uppercased: %v", got)
	}

	got = serialized(ARParentalControlRule{fMac: "AA:BB:CC:DD:EE:FF", fType: pc.ARParentalControlTypeTime})
	if got["MULTIFILTER_MAC"] != "AA:BB:CC:DD:EE:FF" || got["MULTIFILTER_DEVICENAME"] != "" || got["MULTIFILTER_MACFILTER_DAYTIME_V2"] != DefaultTimemap {
		t.Errorf("defaults: %v", got)
	}

	rules := rulesOf(t, map[nvram.ARNvramType]any{
		"MULTIFILTER_MAC":                  rawMac,
		"MULTIFILTER_DEVICENAME":           rawName,
		"MULTIFILTER_ENABLE":               rawType,
		"MULTIFILTER_MACFILTER_DAYTIME_V2": rawTimemap,
	})
	want := map[string]any{
		"MULTIFILTER_MAC":                  "AA:BB:CC:DD:EE:FF>11:22:33:44:55:66",
		"MULTIFILTER_DEVICENAME":           "phone>tablet",
		"MULTIFILTER_ENABLE":               "2>1",
		"MULTIFILTER_MACFILTER_DAYTIME_V2": "W03E21000700<W04122000800>W01E21000700",
	}
	if got := serialized(rules...); !reflect.DeepEqual(got, want) {
		t.Errorf("round_trip: %v", got)
	}
}

func TestScheduleMode(t *testing.T) {
	rules := rulesOf(t, map[nvram.ARNvramType]any{"MULTIFILTER_MAC": "AA:BB:CC:DD:EE:FF", "MULTIFILTER_ENABLE": "1", "MULTIFILTER_MACFILTER_DAYTIME_V2": "M13E17002100"})
	if rules[0][fMode] != modeOnline {
		t.Errorf("parse_online: %v", rules[0])
	}

	got := serialized(ARParentalControlRule{fMac: "AA:BB:CC:DD:EE:FF", fType: pc.ARParentalControlTypeTime, fMode: modeOnline, fTimemap: "W03E21000700<W04122000800"})
	if got["MULTIFILTER_MACFILTER_DAYTIME_V2"] != "M03E21000700<M04122000800" {
		t.Errorf("serialize_mode_field_rewrites_timemap: %v", got)
	}

	got = serialized(ARParentalControlRule{fMac: "AA:BB:CC:DD:EE:FF", fType: pc.ARParentalControlTypeTime, fMode: modeUnknown, fTimemap: "W01E21000700"})
	if got["MULTIFILTER_MACFILTER_DAYTIME_V2"] != "W01E21000700" {
		t.Errorf("serialize_unknown_mode_keeps_timemap: %v", got)
	}

	if RuleScheduleMode(ARParentalControlRule{fMode: modeOnline, fTimemap: "W01E21000700"}) != modeOnline {
		t.Error("rule_schedule_mode_field_wins")
	}
	if RuleScheduleMode(ARParentalControlRule{fTimemap: "M01E17002100"}) != modeOnline || RuleScheduleMode(ARParentalControlRule{}) != modeUnknown {
		t.Error("rule_schedule_mode_from_timemap")
	}
}

func entry(days WeekdaySet, startHour int) ARScheduleEntry {
	e := NewARScheduleEntry()
	e.Days, e.StartHour = days, startHour
	return e
}

func TestScheduleField(t *testing.T) {
	rules := rulesOf(t, map[nvram.ARNvramType]any{"MULTIFILTER_MAC": "AA:BB:CC:DD:EE:FF", "MULTIFILTER_ENABLE": "1", "MULTIFILTER_MACFILTER_DAYTIME_V2": "W03E21000700"})
	schedule := rules[0][fSchedule].([]ARScheduleEntry)
	if len(schedule) != 1 || !schedule[0].Days.Equal(Weekdays) || schedule[0].StartHour != 21 {
		t.Errorf("parsed_rule_exposes_entries: %v", schedule)
	}

	got := serialized(ARParentalControlRule{fMac: "AA:BB:CC:DD:EE:FF", fType: pc.ARParentalControlTypeTime, fSchedule: []ARScheduleEntry{entry(Weekdays, 8)}, fTimemap: "W04122000800"})
	if got["MULTIFILTER_MACFILTER_DAYTIME_V2"] != "W13E08002400" {
		t.Errorf("schedule_wins_over_raw_timemap: %v", got)
	}

	got = serialized(ARParentalControlRule{fMac: "AA:BB:CC:DD:EE:FF", fType: pc.ARParentalControlTypeTime, fMode: modeOnline, fSchedule: []ARScheduleEntry{entry(Weekdays, 8)}})
	if got["MULTIFILTER_MACFILTER_DAYTIME_V2"] != "M13E08002400" {
		t.Errorf("schedule_applies_mode: %v", got)
	}

	if n := RuleEntryCount(ARParentalControlRule{fSchedule: []ARScheduleEntry{entry(Weekdays, 0), entry(Weekend, 0)}}); n != 2 {
		t.Errorf("entry_count_from_schedule: %d", n)
	}
	if n := RuleEntryCount(ARParentalControlRule{fTimemap: "W03E21000700<W04122000800"}); n != 2 {
		t.Errorf("entry_count_from_raw_timemap: %d", n)
	}
	if n := RuleEntryCount(ARParentalControlRule{fMac: "AA:BB:CC:DD:EE:FF"}); n != 2 {
		t.Errorf("entry_count_default: %d", n)
	}
}
