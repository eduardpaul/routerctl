package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	pc "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const (
	cmdAdd    = pc.ARParentalControlCommandAdd
	cmdRemove = pc.ARParentalControlCommandRemove
	cmdSet    = pc.ARParentalControlCommandSet
	cmdState  = pc.ARParentalControlCommandState
)

func macOf(s string) identifiers.MacAddress { return *identifiers.MacAddressFromValueSafe(s) }

func rule(mac, name string, type_ pc.ARParentalControlType) ARParentalControlRule {
	return ARParentalControlRule{fMac: macOf(mac), fName: name, fType: type_, fTimemap: "W01E21000700"}
}

var (
	ruleA = rule("AA:AA:AA:AA:AA:AA", "A", pc.ARParentalControlTypeBlock)
	ruleB = rule("BB:BB:BB:BB:BB:BB", "B", pc.ARParentalControlTypeTime)
	// Same MAC as ruleA, different name - a duplicate as far as the device cares
	ruleADup = rule("AA:AA:AA:AA:AA:AA", "A-again", pc.ARParentalControlTypeDisable)
)

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

func poster(calls *[]call, reply any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return reply, nil
	}
}

var ok = map[string]any{"run_service": "restart_firewall"}

// data is a FetchData returning the source content with the rules
// (rules nil -> an empty dict content, like Python {source: None}).
func data(rules []ARParentalControlRule, calls *int) registry.FetchDataFunc {
	return func(context.Context, source.DataSource, registry.Kwargs) (any, error) {
		if calls != nil {
			*calls++
		}
		if rules == nil {
			return map[F]any(nil), nil
		}
		return map[F]any{fRules: rules}, nil
	}
}

func payload(t *testing.T, calls []call) map[string]any {
	t.Helper()
	if len(calls) == 0 {
		t.Fatal("no request")
	}
	last := calls[len(calls)-1]
	if last.endpoint != endpoint.AREndpointPushData {
		t.Fatalf("endpoint %s", last.endpoint)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(last.request), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func run(t *testing.T, c *registry.Context, a *ARParentalControlAction) bool {
	t.Helper()
	got, err := RunAction(context.Background(), c, a)
	if err != nil {
		t.Fatal(err)
	}
	return got.(service.ARServiceResult).Success
}

func bp(b bool) *bool { return &b }

func TestState(t *testing.T) {
	for state, want := range map[bool]float64{true: 1, false: 0} {
		var calls []call
		success := run(t, &registry.Context{Read: poster(&calls, ok)}, &ARParentalControlAction{Command: cmdState, State: bp(state)})
		p := payload(t, calls)
		if !success || p["rc_service"] != "restart_firewall" || p["MULTIFILTER_ALL"] != want {
			t.Errorf("toggle %v: %v", state, p)
		}
	}
	var calls []call
	if run(t, &registry.Context{Read: poster(&calls, ok)}, &ARParentalControlAction{Command: cmdState}) || len(calls) != 0 {
		t.Error("missing_state")
	}
}

func TestRules(t *testing.T) {
	type tc struct {
		name    string
		action  *ARParentalControlAction
		fetch   registry.FetchDataFunc
		success bool
		check   func(t *testing.T, p map[string]any)
	}
	tests := []tc{
		{name: "unknown_command", action: &ARParentalControlAction{Command: pc.ARParentalControlCommandUnknown}},
		{name: "add", action: &ARParentalControlAction{Command: cmdAdd, Rules: []ARParentalControlRule{ruleB}}, fetch: data([]ARParentalControlRule{ruleA}, nil), success: true,
			check: func(t *testing.T, p map[string]any) {
				if p["MULTIFILTER_MAC"] != "BB:BB:BB:BB:BB:BB>AA:AA:AA:AA:AA:AA" {
					t.Errorf("payload %v", p)
				}
			}},
		{name: "add_replaces_existing_mac", action: &ARParentalControlAction{Command: cmdAdd, Rules: []ARParentalControlRule{ruleADup}}, fetch: data([]ARParentalControlRule{ruleA}, nil), success: true,
			check: func(t *testing.T, p map[string]any) {
				if p["MULTIFILTER_MAC"] != "AA:AA:AA:AA:AA:AA" || p["MULTIFILTER_DEVICENAME"] != "A-again" {
					t.Errorf("payload %v", p)
				}
			}},
		{name: "add_without_data_callback", action: &ARParentalControlAction{Command: cmdAdd, Rules: []ARParentalControlRule{ruleA}}},
		{name: "add_bad_fetch", action: &ARParentalControlAction{Command: cmdAdd, Rules: []ARParentalControlRule{ruleA}},
			fetch: func(context.Context, source.DataSource, registry.Kwargs) (any, error) { return "x", nil }},
		{name: "add_none_data", action: &ARParentalControlAction{Command: cmdAdd, Rules: []ARParentalControlRule{ruleA}}, fetch: data(nil, nil), success: true,
			check: func(t *testing.T, p map[string]any) {
				want := serialized(ruleA)
				want["action_mode"], want["rc_service"] = "apply", "restart_firewall"
				if !reflect.DeepEqual(p, want) {
					t.Errorf("payload %v", p)
				}
			}},
		{name: "add_empty_noop", action: &ARParentalControlAction{Command: cmdAdd}, fetch: data([]ARParentalControlRule{}, nil)},
		{name: "add_unknown_type_rejected", action: &ARParentalControlAction{Command: cmdAdd, Rules: []ARParentalControlRule{{fMac: macOf("AA:AA:AA:AA:AA:AA")}}}, fetch: data([]ARParentalControlRule{}, nil)},
		{name: "set", action: &ARParentalControlAction{Command: cmdSet, Rules: []ARParentalControlRule{ruleA, ruleB}}, success: true,
			check: func(t *testing.T, p map[string]any) {
				if p["MULTIFILTER_MAC"] != "AA:AA:AA:AA:AA:AA>BB:BB:BB:BB:BB:BB" {
					t.Errorf("payload %v", p)
				}
			}},
		{name: "set_empty_clears", action: &ARParentalControlAction{Command: cmdSet}, success: true,
			check: func(t *testing.T, p map[string]any) {
				if p["MULTIFILTER_MAC"] != "" {
					t.Errorf("payload %v", p)
				}
			}},
		{name: "set_incomplete_rejected", action: &ARParentalControlAction{Command: cmdSet, Rules: []ARParentalControlRule{ruleA, {fName: "bad"}}}},
		{name: "remove", action: &ARParentalControlAction{Command: cmdRemove, Rules: []ARParentalControlRule{{fMac: macOf("AA:AA:AA:AA:AA:AA")}}},
			fetch: data([]ARParentalControlRule{ruleA, ruleB}, nil), success: true,
			check: func(t *testing.T, p map[string]any) {
				if p["MULTIFILTER_MAC"] != "BB:BB:BB:BB:BB:BB" {
					t.Errorf("payload %v", p)
				}
			}},
		{name: "remove_without_data_callback", action: &ARParentalControlAction{Command: cmdRemove, Rules: []ARParentalControlRule{{fMac: macOf("AA:AA:AA:AA:AA:AA")}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			success := run(t, &registry.Context{Read: poster(&calls, ok), FetchData: tt.fetch}, tt.action)
			if success != tt.success {
				t.Errorf("success %v", success)
			}
			if !tt.success && len(calls) != 0 {
				t.Errorf("posted %v", calls)
			}
			if tt.check != nil {
				tt.check(t, payload(t, calls))
			}
		})
	}

	// Rejected before reading the current rules
	for name, a := range map[string]*ARParentalControlAction{
		"add_incomplete_rejected": {Command: cmdAdd, Rules: []ARParentalControlRule{{fName: "nope"}}},
		"remove_no_specs":         {Command: cmdRemove, Rules: []ARParentalControlRule{{}}},
	} {
		var calls []call
		fetches := 0
		if run(t, &registry.Context{Read: poster(&calls, ok), FetchData: data([]ARParentalControlRule{ruleA}, &fetches)}, a) || len(calls) != 0 || fetches != 0 {
			t.Errorf("%s: calls %v, fetches %d", name, calls, fetches)
		}
	}
}

func TestExpire(t *testing.T) {
	var calls []call
	expired := map[source.DataSource]bool{}
	expire := func(_ context.Context, src source.DataSource) { expired[src] = true }
	run(t, &registry.Context{Read: poster(&calls, ok), Expire: expire}, &ARParentalControlAction{Command: cmdState, State: bp(true)})
	if !expired[ARParentalControlSourceUniversal] {
		t.Error("source not expired")
	}
	for _, item := range PcRequest {
		if !expired[item] {
			t.Errorf("%s not expired", item)
		}
	}

	expired = map[source.DataSource]bool{}
	run(t, &registry.Context{Read: poster(&calls, nil), Expire: expire}, &ARParentalControlAction{Command: cmdState, State: bp(true)})
	if len(expired) != 0 {
		t.Errorf("expired on failure: %v", expired)
	}
}

func capabilityIdentity(key pc.ARParentalControlCapability, value any) *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(support.ARSupportSourceUniversal): map[support.ARSupportType]any{
			support.ARSupportTypeParentalControlCapabilities: map[pc.ARParentalControlCapability]any{key: value},
		},
	})
}

func rules(count int) []ARParentalControlRule {
	out := make([]ARParentalControlRule, count)
	for i := range out {
		out[i] = rule(fmt.Sprintf("AA:AA:AA:AA:AA:%02X", i), fmt.Sprintf("r%d", i), pc.ARParentalControlTypeBlock)
	}
	return out
}

func TestMaxRules(t *testing.T) {
	limit := func(v any) *device.ARDeviceIdentity {
		return capabilityIdentity(pc.ARParentalControlCapabilityMaxRules, v)
	}
	if maxRules(nil) != DefaultMaxRules || maxRules(limit(0)) != DefaultMaxRules || maxRules(limit("32")) != 32 {
		t.Error("limits")
	}

	tests := []struct {
		name    string
		action  *ARParentalControlAction
		fetch   registry.FetchDataFunc
		success bool
	}{
		{"set_over_limit_rejected", &ARParentalControlAction{Command: cmdSet, Rules: rules(3)}, nil, false},
		{"set_at_limit_allowed", &ARParentalControlAction{Command: cmdSet, Rules: rules(2)}, nil, true},
		{"add_over_limit_rejected", &ARParentalControlAction{Command: cmdAdd, Rules: []ARParentalControlRule{ruleB}}, data(rules(2), nil), false},
		{"remove_ignores_limit", &ARParentalControlAction{Command: cmdRemove, Rules: []ARParentalControlRule{{fName: "r0"}}}, data(rules(3), nil), true},
	}
	for _, tt := range tests {
		var calls []call
		if got := run(t, &registry.Context{Read: poster(&calls, ok), FetchData: tt.fetch, Identity: limit(2)}, tt.action); got != tt.success {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
}

func windowedRule(mac string, windows int) ARParentalControlRule {
	schedule := make([]ARScheduleEntry, windows)
	for i := range schedule {
		schedule[i] = entry(Weekdays, 0)
	}
	return ARParentalControlRule{fMac: macOf(mac), fName: mac, fType: pc.ARParentalControlTypeTime, fSchedule: schedule}
}

func TestMaxEntries(t *testing.T) {
	limit := func(v any) *device.ARDeviceIdentity {
		return capabilityIdentity(pc.ARParentalControlCapabilityMaxEntries, v)
	}
	if maxEntries(nil) != DefaultMaxEntries || maxEntries(limit("64")) != 64 {
		t.Error("limits")
	}
	tests := []struct {
		name    string
		rules   []ARParentalControlRule
		limit   int
		success bool
	}{
		{"over_limit_rejected", []ARParentalControlRule{windowedRule("AA:AA:AA:AA:AA:AA", 3)}, 2, false},
		{"summed_across_rules", []ARParentalControlRule{windowedRule("AA:AA:AA:AA:AA:AA", 2), windowedRule("BB:BB:BB:BB:BB:BB", 2)}, 3, false},
		{"at_limit_allowed", []ARParentalControlRule{windowedRule("AA:AA:AA:AA:AA:AA", 2)}, 2, true},
	}
	for _, tt := range tests {
		var calls []call
		if got := run(t, &registry.Context{Read: poster(&calls, ok), Identity: limit(tt.limit)}, &ARParentalControlAction{Command: cmdSet, Rules: tt.rules}); got != tt.success {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
}

func TestScheduleModeSupport(t *testing.T) {
	online := ARParentalControlRule{fMac: macOf("CC:CC:CC:CC:CC:CC"), fName: "online", fType: pc.ARParentalControlTypeTime, fMode: modeOnline, fTimemap: "M13E17002100"}
	version := func(v any) *device.ARDeviceIdentity {
		return capabilityIdentity(pc.ARParentalControlCapabilitySchedVersion, v)
	}

	var calls []call
	if run(t, &registry.Context{Read: poster(&calls, ok)}, &ARParentalControlAction{Command: cmdSet, Rules: []ARParentalControlRule{online}}) || len(calls) != 0 {
		t.Error("online_rejected_without_support")
	}
	if run(t, &registry.Context{Read: poster(&calls, ok), Identity: version(1)}, &ARParentalControlAction{Command: cmdSet, Rules: []ARParentalControlRule{online}}) || len(calls) != 0 {
		t.Error("online_rejected_below_min_version")
	}
	if !run(t, &registry.Context{Read: poster(&calls, ok), Identity: version(3)}, &ARParentalControlAction{Command: cmdSet, Rules: []ARParentalControlRule{online}}) ||
		payload(t, calls)["MULTIFILTER_MACFILTER_DAYTIME_V2"] != "M13E17002100" {
		t.Error("online_allowed_with_support")
	}
	if !run(t, &registry.Context{Read: poster(&calls, ok)}, &ARParentalControlAction{Command: cmdSet, Rules: []ARParentalControlRule{ruleA}}) {
		t.Error("offline_allowed_without_support")
	}
}

func TestSpecMatches(t *testing.T) {
	if specMatches(ARParentalControlRule{}, ruleA) {
		t.Error("empty_spec_never_matches")
	}
	spec := ARParentalControlRule{fType: pc.ARParentalControlTypeBlock}
	if !specMatches(spec, ruleA) || specMatches(spec, ruleB) {
		t.Error("partial_spec_wildcards")
	}
	if !specMatches(ARParentalControlRule{fName: "A"}, ruleA) || specMatches(ARParentalControlRule{fName: "A"}, ARParentalControlRule{fMac: ruleA[fMac]}) {
		t.Error("plain_field_spec")
	}
}
