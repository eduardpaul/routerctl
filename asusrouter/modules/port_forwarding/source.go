package portforwarding

import (
	"context"
	"reflect"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// KeyState mirrors `KEY_STATE`.
const KeyState = nvram.ARNvramTypePortForwardingState

// Per-WAN rule lists; secondary is only populated on dual-WAN load-balance
// (a slice keeps the Python dict order)
var ruleKeys = []struct {
	unit int
	key  nvram.ARNvramType
}{
	{0, nvram.ARNvramTypePortForwardingList},
	{1, nvram.ARNvramTypePortForwardingListSecondary},
}

// Full NVRAM request for the port forwarding state, built once
var pfRequest = []nvram.ARNvramItem{
	KeyState,
	nvram.ARNvramTypePortForwardingList,
	nvram.ARNvramTypePortForwardingListSecondary,
}

// nvram rule columns: name > external_port > internal_ip > internal_port >
// protocol > source_ip
const (
	colName         = 0
	colExternalPort = 1
	colInternalIP   = 2
	colInternalPort = 3
	colProtocol     = 4
	colSourceIP     = 5
)

// ARPortForwardingSource mirrors `ARPortForwardingSource`: AsusRouter
// port forwarding data source.
type ARPortForwardingSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARPortForwardingSource) SourceName() string { return "ARPortForwardingSource" }

// ARPortForwardingSourceUniversal is the universal instance - preferred.
var ARPortForwardingSourceUniversal = &ARPortForwardingSource{}

// fetchState mirrors `fetch_state`: fetch the port forwarding
// configuration through the NVRAM module.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	return nvramsource.AsyncFetchValues(ctx, c, pfRequest, nil)
}

// parseRule mirrors `_parse_rule`: build a single rule dict from its
// nvram columns.
func parseRule(parts []string, wanUnit int) ARPortForwardingRule {
	col := func(index int) any {
		if field := readers.GetField(parts, index); field != nil {
			return *field
		}
		return nil
	}

	protocol := ARPortForwardingProtocolFromValue(col(colProtocol))

	rule := ARPortForwardingRule{
		ARPortForwardingFieldProtocol: protocol,
		ARPortForwardingFieldWanUnit:  wanUnit,
	}

	if name := converters.RawToStr(col(colName)); name != nil {
		rule[ARPortForwardingFieldName] = *name
	}

	external := converters.RawToStr(col(colExternalPort))
	if protocol == ARPortForwardingProtocolOther {
		// For OTHER the external-port slot carries a raw IP protocol number
		var number *int
		if external != nil {
			number = converters.RawToInt(*external)
		}
		if number != nil {
			rule[ARPortForwardingFieldProtocolNumber] = *number
		}
	} else if external != nil {
		rule[ARPortForwardingFieldExternalPort] = *external
	}

	if internalIP := identifiers.IpAddressFromValueSafe(col(colInternalIP)); internalIP != nil {
		rule[ARPortForwardingFieldInternalIp] = internalIP
	}

	if internalPort := converters.RawToStr(col(colInternalPort)); internalPort != nil {
		rule[ARPortForwardingFieldInternalPort] = *internalPort
	}

	if sourceIP := identifiers.IpInterfaceFromValueSafe(col(colSourceIP)); sourceIP != nil {
		rule[ARPortForwardingFieldSourceIp] = sourceIP
	}

	return rule
}

// parseRules mirrors `_parse_rules`: parse one WAN's rule list into rule
// dicts.
func parseRules(raw any, wanUnit int) []ARPortForwardingRule {
	rules := []ARPortForwardingRule{}
	for _, row := range readers.SplitRows(raw) {
		if row == "" {
			continue
		}
		rules = append(rules, parseRule(strings.Split(row, ">"), wanUnit))
	}

	return rules
}

// isNone reports Python `value is None` (nil or a nil pointer).
func isNone(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return rv.IsNil()
	}
	return false
}

// pyStr mirrors Python `str(value)`, rendering enums by their value.
func pyStr(value any) string {
	if p, ok := value.(ARPortForwardingProtocol); ok {
		return string(p)
	}
	return security.PyStrOf(value)
}

// ruleColumns mirrors `_rule_columns`: serialize a rule dict back into
// its ordered nvram columns.
func ruleColumns(rule ARPortForwardingRule) []string {
	protocol := ARPortForwardingProtocolFromValue(rule[ARPortForwardingFieldProtocol])
	var external any
	if protocol == ARPortForwardingProtocolOther {
		external = rule[ARPortForwardingFieldProtocolNumber]
	} else {
		external = rule[ARPortForwardingFieldExternalPort]
	}

	// In nvram column order: name, external, internal_ip, internal_port,
	// protocol, source_ip
	values := []any{
		rule[ARPortForwardingFieldName],
		external,
		rule[ARPortForwardingFieldInternalIp],
		rule[ARPortForwardingFieldInternalPort],
		protocol,
		rule[ARPortForwardingFieldSourceIp],
	}
	out := make([]string, len(values))
	for i, value := range values {
		if !isNone(value) {
			out[i] = pyStr(value)
		}
	}
	return out
}

// SerializeRules mirrors `serialize_rules`: serialize a list of rule
// dicts into the raw nvram list string.
func SerializeRules(rules []ARPortForwardingRule) string {
	var b strings.Builder
	for _, rule := range rules {
		b.WriteString("<")
		b.WriteString(strings.Join(ruleColumns(rule), ">"))
	}
	return b.String()
}

// translateState mirrors `translate_state`: translate raw port
// forwarding nvram into a structured dict. The data is keyed by NVRAM
// item ID (the nvram fetch result).
func translateState(raw any, tc *registry.TranslateContext) any {
	data, ok := raw.(map[source.ID]any)
	if !ok || len(data) == 0 {
		return map[ARPortForwardingField]any{}
	}

	rules := []ARPortForwardingRule{}
	for _, rk := range ruleKeys {
		rules = append(rules, parseRules(data[source.IDOf(rk.key)], rk.unit)...)
	}

	state := converters.RawToBool(data[source.IDOf(KeyState)])
	return map[ARPortForwardingField]any{
		ARPortForwardingFieldState: state != nil && *state,
		ARPortForwardingFieldRules: rules,
	}
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARPortForwardingSource",
		Default:   func() source.DataSource { return &ARPortForwardingSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
