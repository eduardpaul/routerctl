package portforwarding

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

var (
	ruleA = ARPortForwardingRule{
		ARPortForwardingFieldName:         "A",
		ARPortForwardingFieldProtocol:     ARPortForwardingProtocolTcp,
		ARPortForwardingFieldExternalPort: "80",
		ARPortForwardingFieldInternalIp:   "192.168.1.10",
	}
	ruleB = ARPortForwardingRule{
		ARPortForwardingFieldName:         "B",
		ARPortForwardingFieldProtocol:     ARPortForwardingProtocolUdp,
		ARPortForwardingFieldExternalPort: "53",
		ARPortForwardingFieldInternalIp:   "192.168.1.11",
	}
	ruleOther = ARPortForwardingRule{
		ARPortForwardingFieldName:           "G",
		ARPortForwardingFieldProtocol:       ARPortForwardingProtocolOther,
		ARPortForwardingFieldProtocolNumber: 47,
		ARPortForwardingFieldInternalIp:     "192.168.1.12",
	}
	ruleSecondary = ARPortForwardingRule{
		ARPortForwardingFieldName:         "S",
		ARPortForwardingFieldProtocol:     ARPortForwardingProtocolTcp,
		ARPortForwardingFieldExternalPort: "22",
		ARPortForwardingFieldInternalIp:   "192.168.1.13",
		ARPortForwardingFieldWanUnit:      1,
	}
	ruleC = ARPortForwardingRule{
		ARPortForwardingFieldName:         "C",
		ARPortForwardingFieldProtocol:     ARPortForwardingProtocolTcp,
		ARPortForwardingFieldExternalPort: "81",
		ARPortForwardingFieldInternalIp:   "192.168.1.14",
	}
	ruleSrc = ARPortForwardingRule{
		ARPortForwardingFieldName:         "D",
		ARPortForwardingFieldProtocol:     ARPortForwardingProtocolUdp,
		ARPortForwardingFieldExternalPort: "443",
		ARPortForwardingFieldInternalIp:   "192.168.1.15",
		ARPortForwardingFieldSourceIp:     "10.0.0.0/24",
	}
	// Same device key as ruleA (unit 0, TCP, port 80, no source) but a
	// different name and internal IP - a duplicate as far as the device cares
	ruleADup = ARPortForwardingRule{
		ARPortForwardingFieldName:         "A-again",
		ARPortForwardingFieldProtocol:     ARPortForwardingProtocolTcp,
		ARPortForwardingFieldExternalPort: "80",
		ARPortForwardingFieldInternalIp:   "192.168.1.99",
	}
	// ruleA restricted to a source - a distinct rule, not a duplicate
	ruleASrc = ARPortForwardingRule{
		ARPortForwardingFieldName:         "A-src",
		ARPortForwardingFieldProtocol:     ARPortForwardingProtocolTcp,
		ARPortForwardingFieldExternalPort: "80",
		ARPortForwardingFieldInternalIp:   "192.168.1.10",
		ARPortForwardingFieldSourceIp:     "10.0.0.0/24",
	}
)

type rec struct {
	calls []struct {
		ep      endpoint.AREndpoint
		request string
	}
	expired []source.DataSource
	fetched int
}

// poster mirrors `_poster`: the push reports the firewall restart ran.
func (r *rec) poster(reply any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		r.calls = append(r.calls, struct {
			ep      endpoint.AREndpoint
			request string
		}{ep, request})
		return reply, nil
	}
}

var ran = map[string]any{"run_service": "restart_firewall"}

// data mirrors `_data`: a get-data callback for the given current rules.
// Go FetchData returns the content directly: nil rules give a typed nil
// map (Python `{source: None}`).
func (r *rec) data(rules []ARPortForwardingRule, none bool) registry.FetchDataFunc {
	return func(context.Context, source.DataSource, registry.Kwargs) (any, error) {
		r.fetched++
		if none {
			return map[ARPortForwardingField]any(nil), nil
		}
		return map[ARPortForwardingField]any{ARPortForwardingFieldRules: rules}, nil
	}
}

// badData mirrors a non-dict fetch result: nothing fresh.
func (r *rec) badData() registry.FetchDataFunc {
	return func(context.Context, source.DataSource, registry.Kwargs) (any, error) {
		r.fetched++
		return nil, nil
	}
}

func run(t *testing.T, c *registry.Context, a *ARPortForwardingAction) service.ARServiceResult {
	t.Helper()
	got, err := runAction(context.Background(), c, a)
	if err != nil {
		t.Fatal(err)
	}
	return got.(service.ARServiceResult)
}

// payload mirrors `_payload`: decode the pushed request body.
func (r *rec) payload(t *testing.T) map[string]any {
	t.Helper()
	if len(r.calls) != 1 {
		t.Fatalf("want one push, got %d", len(r.calls))
	}
	if r.calls[0].ep != endpoint.AREndpointPushData {
		t.Fatalf("endpoint = %s", r.calls[0].ep)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(r.calls[0].request), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rec) request(t *testing.T) string {
	t.Helper()
	if len(r.calls) != 1 {
		t.Fatalf("want one push, got %d", len(r.calls))
	}
	return r.calls[0].request
}

func rules(rs ...ARPortForwardingRule) []ARPortForwardingRule { return rs }

func boolp(b bool) *bool { return &b }

// ---- TestState ----

func TestStateToggle(t *testing.T) {
	cases := []struct {
		state bool
		want  float64
		exact string
	}{
		{true, 1, `{"action_mode":"apply","rc_service":"restart_firewall","vts_enable_x":1}`},
		{false, 0, `{"action_mode":"apply","rc_service":"restart_firewall","vts_enable_x":0}`},
	}
	for _, tc := range cases {
		r := &rec{}
		res := run(t, &registry.Context{Read: r.poster(ran)},
			&ARPortForwardingAction{Command: ARPortForwardingCommandState, State: boolp(tc.state)})
		p := r.payload(t)
		if p["rc_service"] != "restart_firewall" || p["vts_enable_x"] != tc.want || !res.Success {
			t.Fatalf("payload = %v, result = %+v", p, res)
		}
		if got := r.request(t); got != tc.exact {
			t.Fatalf("request = %s, want %s", got, tc.exact)
		}
	}
}

func TestStateMissing(t *testing.T) {
	r := &rec{}
	res := run(t, &registry.Context{Read: r.poster(ran)}, &ARPortForwardingAction{Command: ARPortForwardingCommandState})
	if res.Success || len(r.calls) != 0 {
		t.Fatalf("result = %+v, calls = %d", res, len(r.calls))
	}
}

// ---- TestRules ----

// rulesCase runs one rule command and checks the pushed list(s).
type rulesCase struct {
	name    string
	action  *ARPortForwardingAction
	current []ARPortForwardingRule
	// noData: no fetch_data_callback; bad: a non-dict fetch; none: a None payload
	noData, bad, none bool
	// want is the expected vts_rulelist (nil: the action must fail without a push)
	want *string
	// exact is the exact pushed body, when pinned
	exact string
	// noFetch asserts the data callback was not awaited
	noFetch bool
}

func sp(s string) *string { return &s }

func TestRulesCommands(t *testing.T) {
	cases := []rulesCase{
		{name: "unknown_command", action: &ARPortForwardingAction{Command: ARPortForwardingCommandUnknown}, noData: true},
		{name: "add", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd, Rules: rules(ruleB)},
			current: rules(ruleA), want: sp(SerializeRules(rules(ruleA, ruleB))),
			exact: `{"action_mode":"apply","rc_service":"restart_firewall","vts_rulelist":"<A>80>192.168.1.10>>TCP><B>53>192.168.1.11>>UDP>"}`},
		{name: "add_without_data_callback", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd, Rules: rules(ruleA)}, noData: true},
		{name: "add_bad_fetch", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd, Rules: rules(ruleA)}, bad: true},
		{name: "add_none_data", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd, Rules: rules(ruleA)},
			none: true, want: sp(SerializeRules(rules(ruleA)))},
		{name: "add_empty_noop", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd}, current: rules()},
		{name: "add_incomplete_rejected", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldInternalIp: "192.168.1.10"})}, current: rules(), noFetch: true},
		{name: "add_other_missing_number_rejected", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldProtocol: ARPortForwardingProtocolOther, ARPortForwardingFieldInternalIp: "192.168.1.10"})},
			current: rules()},
		{name: "add_missing_ip_rejected", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldProtocol: ARPortForwardingProtocolTcp, ARPortForwardingFieldExternalPort: "80"})},
			current: rules()},
		{name: "remove_bad_fetch", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldInternalIp: "192.168.1.10"})}, bad: true},
		{name: "set_incomplete_rejected", action: &ARPortForwardingAction{Command: ARPortForwardingCommandSet,
			Rules: rules(ruleA, ARPortForwardingRule{ARPortForwardingFieldInternalIp: "192.168.1.10"})}, noData: true},
		{name: "set_other_complete", action: &ARPortForwardingAction{Command: ARPortForwardingCommandSet, Rules: rules(ruleOther)},
			noData: true, want: sp(SerializeRules(rules(ruleOther)))},
		{name: "add_duplicate_of_saved_skipped", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd, Rules: rules(ruleADup)},
			current: rules(ruleA), want: sp(SerializeRules(rules(ruleA)))},
		{name: "saved_duplicates_cleaned", action: &ARPortForwardingAction{Command: ARPortForwardingCommandAdd, Rules: rules(ruleB)},
			current: rules(ruleA, ruleADup), want: sp(SerializeRules(rules(ruleA, ruleB)))},
		{name: "distinct_source_not_deduped", action: &ARPortForwardingAction{Command: ARPortForwardingCommandSet, Rules: rules(ruleA, ruleASrc)},
			noData: true, want: sp(SerializeRules(rules(ruleA, ruleASrc)))},
		{name: "dedup_other_by_protocol_number", action: &ARPortForwardingAction{Command: ARPortForwardingCommandSet,
			Rules: rules(ruleOther, ARPortForwardingRule{
				ARPortForwardingFieldName:           "G2",
				ARPortForwardingFieldProtocol:       ARPortForwardingProtocolOther,
				ARPortForwardingFieldProtocolNumber: 47,
				ARPortForwardingFieldInternalIp:     "192.168.1.9",
			})}, noData: true, want: sp(SerializeRules(rules(ruleOther)))},
		{name: "remove_exact_read_rule", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove, Rules: rules(ruleA)},
			current: rules(ruleA, ruleB), want: sp(SerializeRules(rules(ruleB)))},
		{name: "remove_wildcard_by_port", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldExternalPort: "80"})},
			current: rules(ruleA, ruleB), want: sp(SerializeRules(rules(ruleB)))},
		{name: "remove_wildcard_by_protocol_multi", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldProtocol: ARPortForwardingProtocolTcp})},
			current: rules(ruleA, ruleC, ruleB), want: sp(SerializeRules(rules(ruleB)))},
		{name: "remove_wildcard_ip_and_port", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldInternalIp: "192.168.1.10", ARPortForwardingFieldExternalPort: "80"})},
			current: rules(ruleA, ruleC), want: sp(SerializeRules(rules(ruleC)))},
		{name: "remove_wildcard_by_source_ip", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldSourceIp: "10.0.0.0/24"})},
			current: rules(ruleSrc, ruleB), want: sp(SerializeRules(rules(ruleB)))},
		{name: "remove_no_specs_rejected", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ARPortForwardingRule{})}, current: rules(ruleA, ruleB)},
		{name: "remove_other_by_protocol_number", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove, Rules: rules(ruleOther)},
			current: rules(ruleOther, ruleB), want: sp(SerializeRules(rules(ruleB)))},
		{name: "remove_by_ip", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldInternalIp: "192.168.1.10"})},
			current: rules(ruleA, ruleB), want: sp(SerializeRules(rules(ruleB)))},
		{name: "remove_by_ip_accepts_ipaddress", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ARPortForwardingRule{ARPortForwardingFieldInternalIp: identifiers.IpAddressFromValueSafe("192.168.1.10")})},
			current: rules(ruleA, ruleB), want: sp(SerializeRules(rules(ruleB)))},
		{name: "remove_multiple_specs", action: &ARPortForwardingAction{Command: ARPortForwardingCommandRemove,
			Rules: rules(ruleA, ARPortForwardingRule{ARPortForwardingFieldInternalIp: "192.168.1.11"})},
			current: rules(ruleA, ruleB), want: sp(""),
			exact: `{"action_mode":"apply","rc_service":"restart_firewall","vts_rulelist":""}`},
		{name: "set_replaces", action: &ARPortForwardingAction{Command: ARPortForwardingCommandSet, Rules: rules(ruleA)},
			noData: true, want: sp(SerializeRules(rules(ruleA)))},
		{name: "set_empty_clears_primary", action: &ARPortForwardingAction{Command: ARPortForwardingCommandSet},
			noData: true, want: sp("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &rec{}
			c := &registry.Context{Read: r.poster(ran)}
			switch {
			case tc.noData:
			case tc.bad:
				c.FetchData = r.badData()
			default:
				c.FetchData = r.data(tc.current, tc.none)
			}
			res := run(t, c, tc.action)
			if tc.want == nil {
				if res.Success || len(r.calls) != 0 {
					t.Fatalf("result = %+v, calls = %d", res, len(r.calls))
				}
				if tc.noFetch && r.fetched != 0 {
					t.Fatal("data callback was awaited")
				}
				return
			}
			if got := r.payload(t)["vts_rulelist"]; got != *tc.want {
				t.Fatalf("vts_rulelist = %q, want %q", got, *tc.want)
			}
			if !res.Success {
				t.Fatalf("result = %+v", res)
			}
			if tc.exact != "" && r.request(t) != tc.exact {
				t.Fatalf("request = %s, want %s", r.request(t), tc.exact)
			}
		})
	}
}

func TestSetSecondaryWritesBoth(t *testing.T) {
	r := &rec{}
	run(t, &registry.Context{Read: r.poster(ran)}, &ARPortForwardingAction{Command: ARPortForwardingCommandSet, Rules: rules(ruleSecondary)})
	p := r.payload(t)
	if p["vts_rulelist"] != "" || p["vts1_rulelist"] != SerializeRules(rules(ruleSecondary)) {
		t.Fatalf("payload = %v", p)
	}
	exact := `{"action_mode":"apply","rc_service":"restart_firewall","vts_rulelist":"","vts1_rulelist":"<S>22>192.168.1.13>>TCP>"}`
	if r.request(t) != exact {
		t.Fatalf("request = %s, want %s", r.request(t), exact)
	}
}

func TestRawCallbackIsPoster(t *testing.T) {
	r, raw := &rec{}, 0
	body := `{"run_service":"restart_firewall"}`
	c := &registry.Context{
		Read: r.poster(ran),
		FetchRaw: func(context.Context, endpoint.AREndpoint, string) (*string, error) {
			raw++
			return &body, nil
		},
	}
	run(t, c, &ARPortForwardingAction{Command: ARPortForwardingCommandState, State: boolp(true)})
	if raw != 1 || len(r.calls) != 0 {
		t.Fatalf("raw = %d, callback = %d", raw, len(r.calls))
	}
}

// ---- TestExpire ----

func TestExpireCalledOnSuccess(t *testing.T) {
	r := &rec{}
	c := &registry.Context{
		Read:   r.poster(ran),
		Expire: func(_ context.Context, src source.DataSource) { r.expired = append(r.expired, src) },
	}
	run(t, c, &ARPortForwardingAction{Command: ARPortForwardingCommandState, State: boolp(true)})
	if len(r.expired) != 1 || r.expired[0] != ARPortForwardingSourceUniversal {
		t.Fatalf("expired = %v", r.expired)
	}
}

func TestNoExpireOnFailedPush(t *testing.T) {
	r := &rec{}
	c := &registry.Context{
		Read:   r.poster(nil),
		Expire: func(_ context.Context, src source.DataSource) { r.expired = append(r.expired, src) },
	}
	res := run(t, c, &ARPortForwardingAction{Command: ARPortForwardingCommandState, State: boolp(true)})
	if res.Success || len(r.expired) != 0 {
		t.Fatalf("result = %+v, expired = %v", res, r.expired)
	}
}

func TestSpecMatchesEmptyNeverMatches(t *testing.T) {
	if specMatches(ARPortForwardingRule{}, ruleA) {
		t.Fatal("empty spec matched")
	}
}

func TestActionRegistered(t *testing.T) {
	if e := registry.Action(&ARPortForwardingAction{}); e == nil || e.Run == nil {
		t.Fatal("ARPortForwardingAction is not registered")
	}
}
