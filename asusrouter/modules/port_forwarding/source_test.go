package portforwarding

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// Encoded nvram rule rows: `<name>ext>internal_ip>internal_port>proto>source`
const (
	ruleTCPRaw    = "&#60Web&#6280&#62192.168.1.10&#628080&#62TCP&#62"
	ruleOtherRaw  = "&#60GRE&#6247&#62192.168.1.20&#62&#62OTHER&#62"
	ruleSourceRaw = "&#60&#62443&#62192.168.1.30&#62&#62UDP&#6210.0.0.0/24"
)

// nv builds nvram data keyed like the nvram fetch result (item ID).
func nv(pairs ...any) map[source.ID]any {
	out := map[source.ID]any{}
	for i := 0; i < len(pairs); i += 2 {
		out[source.IDOf(pairs[i].(nvram.ARNvramItem))] = pairs[i+1]
	}
	return out
}

func translated(t *testing.T, data any) map[ARPortForwardingField]any {
	t.Helper()
	got, ok := translateState(data, nil).(map[ARPortForwardingField]any)
	if !ok {
		t.Fatalf("translateState returned %T", got)
	}
	return got
}

func onlyRule(t *testing.T, data any) ARPortForwardingRule {
	t.Helper()
	rules := translated(t, data)[ARPortForwardingFieldRules].([]ARPortForwardingRule)
	if len(rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rules))
	}
	return rules[0]
}

func str(v any) string { return pyStr(v) }

// ---- TestGetState ----

func TestFetchStateViaNvram(t *testing.T) {
	values := nv(nvram.ARNvramTypePortForwardingState, "1")
	var requested []source.DataSource
	readCalled := false
	c := &registry.Context{
		Read: func(context.Context, endpoint.AREndpoint, string) (any, error) {
			readCalled = true
			return nil, nil
		},
		FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
			requested = srcs
			return values, nil
		},
	}
	got, err := fetchState(context.Background(), c, ARPortForwardingSourceUniversal)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, values) {
		t.Fatalf("fetchState = %v, want %v", got, values)
	}
	if readCalled {
		t.Fatal("callback was awaited")
	}
	want := []source.DataSource{
		nvram.ARNvramTypePortForwardingState,
		nvram.ARNvramTypePortForwardingList,
		nvram.ARNvramTypePortForwardingListSecondary,
	}
	if !reflect.DeepEqual(requested, want) {
		t.Fatalf("request = %v, want %v", requested, want)
	}
}

func TestFetchStateNoDataCallback(t *testing.T) {
	got, err := fetchState(context.Background(), &registry.Context{}, ARPortForwardingSourceUniversal)
	if err != nil || len(got.(map[source.ID]any)) != 0 {
		t.Fatalf("fetchState = %v, %v", got, err)
	}
}

func TestFetchStateNonDictResponse(t *testing.T) {
	c := &registry.Context{
		FetchDataMulti: func(context.Context, []source.DataSource, registry.Kwargs) (map[source.ID]any, error) {
			return nil, nil
		},
	}
	got, err := fetchState(context.Background(), c, ARPortForwardingSourceUniversal)
	if err != nil || len(got.(map[source.ID]any)) != 0 {
		t.Fatalf("fetchState = %v, %v", got, err)
	}
}

// ---- TestTranslateState ----

func TestTranslateEmpty(t *testing.T) {
	for _, data := range []any{nil, map[source.ID]any{}, "text", []any{}} {
		if got := translated(t, data); len(got) != 0 {
			t.Fatalf("translateState(%v) = %v", data, got)
		}
	}
}

func TestTranslateStateFlag(t *testing.T) {
	cases := []struct {
		name string
		data map[source.ID]any
		want bool
	}{
		{"1", nv(nvram.ARNvramTypePortForwardingState, "1"), true},
		{"0", nv(nvram.ARNvramTypePortForwardingState, "0"), false},
		{"none", nv(nvram.ARNvramTypePortForwardingList, ""), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translated(t, tc.data)
			if got[ARPortForwardingFieldState] != tc.want {
				t.Fatalf("state = %v", got[ARPortForwardingFieldState])
			}
			if rules := got[ARPortForwardingFieldRules].([]ARPortForwardingRule); len(rules) != 0 {
				t.Fatalf("rules = %v", rules)
			}
		})
	}
}

func TestTranslateTCPRule(t *testing.T) {
	rule := onlyRule(t, nv(nvram.ARNvramTypePortForwardingList, ruleTCPRaw))
	if rule[ARPortForwardingFieldName] != "Web" ||
		rule[ARPortForwardingFieldProtocol] != ARPortForwardingProtocolTcp ||
		rule[ARPortForwardingFieldExternalPort] != "80" ||
		str(rule[ARPortForwardingFieldInternalIp]) != "192.168.1.10" ||
		rule[ARPortForwardingFieldInternalPort] != "8080" ||
		rule[ARPortForwardingFieldWanUnit] != 0 {
		t.Fatalf("rule = %v", rule)
	}
	for _, f := range []ARPortForwardingField{ARPortForwardingFieldSourceIp, ARPortForwardingFieldProtocolNumber} {
		if _, ok := rule[f]; ok {
			t.Fatalf("%s present", f)
		}
	}
}

func TestTranslateOtherRule(t *testing.T) {
	rule := onlyRule(t, nv(nvram.ARNvramTypePortForwardingList, ruleOtherRaw))
	if rule[ARPortForwardingFieldProtocol] != ARPortForwardingProtocolOther || rule[ARPortForwardingFieldProtocolNumber] != 47 {
		t.Fatalf("rule = %v", rule)
	}
	for _, f := range []ARPortForwardingField{ARPortForwardingFieldExternalPort, ARPortForwardingFieldInternalPort} {
		if _, ok := rule[f]; ok {
			t.Fatalf("%s present", f)
		}
	}
}

func TestTranslateOtherNonNumeric(t *testing.T) {
	rule := onlyRule(t, nv(nvram.ARNvramTypePortForwardingList, "&#60x&#62abc&#62192.168.1.20&#62&#62OTHER&#62"))
	if _, ok := rule[ARPortForwardingFieldProtocolNumber]; ok {
		t.Fatal("protocol_number present")
	}
}

func TestTranslateSourceIP(t *testing.T) {
	rule := onlyRule(t, nv(nvram.ARNvramTypePortForwardingList, ruleSourceRaw))
	if str(rule[ARPortForwardingFieldSourceIp]) != "10.0.0.0/24" {
		t.Fatalf("source_ip = %v", rule[ARPortForwardingFieldSourceIp])
	}
	for _, f := range []ARPortForwardingField{ARPortForwardingFieldName, ARPortForwardingFieldInternalPort} {
		if _, ok := rule[f]; ok {
			t.Fatalf("%s present", f)
		}
	}
}

func TestTranslateBadInternalIPOmitted(t *testing.T) {
	rule := onlyRule(t, nv(nvram.ARNvramTypePortForwardingList, "&#60x&#6280&#62nope&#628080&#62TCP&#62"))
	if _, ok := rule[ARPortForwardingFieldInternalIp]; ok {
		t.Fatal("internal_ip present")
	}
}

func TestTranslateShortRow(t *testing.T) {
	rule := onlyRule(t, nv(nvram.ARNvramTypePortForwardingList, "&#60x&#6280"))
	if rule[ARPortForwardingFieldProtocol] != ARPortForwardingProtocolUnknown {
		t.Fatalf("protocol = %v", rule[ARPortForwardingFieldProtocol])
	}
}

func TestTranslateSecondaryWan(t *testing.T) {
	got := translated(t, nv(
		nvram.ARNvramTypePortForwardingList, ruleTCPRaw,
		nvram.ARNvramTypePortForwardingListSecondary, ruleOtherRaw,
	))
	var units []any
	for _, r := range got[ARPortForwardingFieldRules].([]ARPortForwardingRule) {
		units = append(units, r[ARPortForwardingFieldWanUnit])
	}
	if !reflect.DeepEqual(units, []any{0, 1}) {
		t.Fatalf("units = %v", units)
	}
}

func TestTranslateNonStringList(t *testing.T) {
	got := translated(t, nv(nvram.ARNvramTypePortForwardingState, "1", nvram.ARNvramTypePortForwardingList, 123))
	if rules := got[ARPortForwardingFieldRules].([]ARPortForwardingRule); len(rules) != 0 {
		t.Fatalf("rules = %v", rules)
	}
}

// ---- TestSerializeRules ----

func TestSerializeRules(t *testing.T) {
	cases := []struct {
		name  string
		rules []ARPortForwardingRule
		want  string
	}{
		{"empty", nil, ""},
		{"tcp_rule", []ARPortForwardingRule{{
			ARPortForwardingFieldName:         "Web",
			ARPortForwardingFieldProtocol:     ARPortForwardingProtocolTcp,
			ARPortForwardingFieldExternalPort: "80",
			ARPortForwardingFieldInternalIp:   "192.168.1.10",
			ARPortForwardingFieldInternalPort: "8080",
		}}, "<Web>80>192.168.1.10>8080>TCP>"},
		{"other_uses_protocol_number", []ARPortForwardingRule{{
			ARPortForwardingFieldName:           "GRE",
			ARPortForwardingFieldProtocol:       ARPortForwardingProtocolOther,
			ARPortForwardingFieldProtocolNumber: 47,
			ARPortForwardingFieldInternalIp:     "192.168.1.20",
		}}, "<GRE>47>192.168.1.20>>OTHER>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SerializeRules(tc.rules); got != tc.want {
				t.Fatalf("SerializeRules = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	for _, raw := range []string{ruleTCPRaw, ruleOtherRaw, ruleSourceRaw} {
		rules := translated(t, nv(nvram.ARNvramTypePortForwardingList, raw))[ARPortForwardingFieldRules].([]ARPortForwardingRule)
		decoded := strings.ReplaceAll(strings.ReplaceAll(raw, "&#60", "<"), "&#62", ">")
		if got := SerializeRules(rules); got != decoded {
			t.Fatalf("round trip = %q, want %q", got, decoded)
		}
	}
}

// ---- test_enums.py ----

func TestEnums(t *testing.T) {
	for _, m := range ARPortForwardingCommandMembers {
		if ARPortForwardingCommandFromValue(string(m)) != m {
			t.Fatalf("command %s", m.Name())
		}
	}
	for _, m := range ARPortForwardingProtocolMembers {
		if ARPortForwardingProtocolFromValue(string(m)) != m {
			t.Fatalf("protocol %s", m.Name())
		}
	}
	for _, m := range ARPortForwardingFieldMembers {
		if ARPortForwardingFieldFromValue(string(m)) != m {
			t.Fatalf("field %s", m.Name())
		}
	}
	const bogus = "zzz-not-a-real-enum-value"
	if ARPortForwardingCommandFromValue(bogus) != ARPortForwardingCommandUnknown ||
		ARPortForwardingProtocolFromValue(bogus) != ARPortForwardingProtocolUnknown ||
		ARPortForwardingFieldFromValue(bogus) != ARPortForwardingFieldUnknown {
		t.Fatal("unknown fallback")
	}
}
