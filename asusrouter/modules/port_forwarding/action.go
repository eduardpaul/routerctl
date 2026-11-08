package portforwarding

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// ARPortForwardingRule mirrors `ARPortForwardingRule`: a rule shares the
// shape the read source emits, for a clean round-trip.
type ARPortForwardingRule = map[ARPortForwardingField]any

// nvram rule-list key per WAN unit
var listKeys = map[int]string{
	0: string(nvram.ARNvramTypePortForwardingList),
	1: string(nvram.ARNvramTypePortForwardingListSecondary),
}

// ARPortForwardingAction mirrors `ARPortForwardingAction`: mutate port
// forwarding rules or the master toggle.
type ARPortForwardingAction struct {
	action.Base

	Command ARPortForwardingCommand
	State   *bool
	// ADD: rules to append. SET: the full replacement list. Both require
	// complete rules. REMOVE: match specs - a current rule is dropped when a
	// spec matches it on every field the spec sets, so a full read rule is an
	// exact match and a partial one (e.g. `{INTERNAL_IP: ...}`) is a wildcard
	Rules []ARPortForwardingRule
}

// ActionName returns the Python class name.
func (*ARPortForwardingAction) ActionName() string { return "ARPortForwardingAction" }

// Rule fields with a defined value when absent from a rule dict
var fieldDefaults = map[ARPortForwardingField]any{
	ARPortForwardingFieldWanUnit: 0,
}

// get mirrors `rule.get(field, default)`.
func get(rule ARPortForwardingRule, field ARPortForwardingField, def any) any {
	if value, ok := rule[field]; ok {
		return value
	}
	return def
}

// norm mirrors `_norm`: normalize a rule field to a comparable string.
func norm(field ARPortForwardingField, value any) string {
	switch field {
	case ARPortForwardingFieldProtocol:
		return string(ARPortForwardingProtocolFromValue(value))
	case ARPortForwardingFieldInternalIp:
		if ip := identifiers.IpAddressFromValueSafe(value); ip != nil {
			return ip.String()
		}
		return pyStr(value)
	case ARPortForwardingFieldSourceIp:
		if iface := identifiers.IpInterfaceFromValueSafe(value); iface != nil {
			return iface.String()
		}
		return pyStr(value)
	}
	if isNone(value) {
		return ""
	}
	return pyStr(value)
}

// specMatches mirrors `_spec_matches`: whether a rule matches a spec on
// every field the spec sets.
func specMatches(spec ARPortForwardingRule, rule ARPortForwardingRule) bool {
	if len(spec) == 0 {
		// An empty spec must not match everything
		return false
	}
	for field, value := range spec {
		if norm(field, value) != norm(field, get(rule, field, fieldDefaults[field])) {
			return false
		}
	}
	return true
}

// unitKey normalizes a WAN unit like Python hashing/equality does
// (`0 == 0.0 == False`): integral numbers and bools become an int.
func unitKey(value any) (int, bool) {
	switch v := value.(type) {
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	case float64:
		if v == math.Trunc(v) && !math.IsInf(v, 0) {
			return int(v), true
		}
		return 0, false
	case float32:
		return unitKey(float64(v))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		i := converters.RawToInt(v)
		if i != nil {
			return *i, true
		}
	}
	return 0, false
}

// unitID renders a WAN unit as a comparable key part.
func unitID(value any) string {
	if i, ok := unitKey(value); ok {
		return fmt.Sprintf("int:%d", i)
	}
	return fmt.Sprintf("%T:%s", value, converters.PyRepr(value))
}

// dedupKey mirrors `_dedup_key`: device identity of a rule; two with the
// same key forward the same port.
//
// Mirrors the web UI's duplicate check - name, internal IP and internal port
// are not part of it, since one external port maps to a single forward.
func dedupKey(rule ARPortForwardingRule) [4]string {
	protocol := ARPortForwardingProtocolFromValue(rule[ARPortForwardingFieldProtocol])
	var external any
	if protocol == ARPortForwardingProtocolOther {
		external = rule[ARPortForwardingFieldProtocolNumber]
	} else {
		external = rule[ARPortForwardingFieldExternalPort]
	}

	externalStr := ""
	if !isNone(external) {
		externalStr = pyStr(external)
	}
	return [4]string{
		unitID(get(rule, ARPortForwardingFieldWanUnit, 0)),
		string(protocol),
		externalStr,
		norm(ARPortForwardingFieldSourceIp, rule[ARPortForwardingFieldSourceIp]),
	}
}

// dedup mirrors `_dedup`: drop duplicate rules, keeping the first
// occurrence of each.
func dedup(rules []ARPortForwardingRule) []ARPortForwardingRule {
	seen := map[[4]string]bool{}
	unique := []ARPortForwardingRule{}
	for _, rule := range rules {
		key := dedupKey(rule)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, rule)
	}
	return unique
}

// truthy mirrors Python `bool(value)` for rule values.
func truthy(value any) bool {
	if isNone(value) {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case *string:
		return *v != ""
	case float64:
		return v != 0
	case float32:
		return v != 0
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		i, _ := unitKey(v)
		return i != 0
	}
	return true
}

// isComplete mirrors `_is_complete`: whether a rule carries the fields the
// device needs to store it.
func isComplete(rule ARPortForwardingRule) bool {
	protocol := ARPortForwardingProtocolFromValue(rule[ARPortForwardingFieldProtocol])
	if protocol == ARPortForwardingProtocolUnknown {
		return false
	}
	if isNone(rule[ARPortForwardingFieldInternalIp]) {
		return false
	}
	if protocol == ARPortForwardingProtocolOther {
		return !isNone(rule[ARPortForwardingFieldProtocolNumber])
	}
	return truthy(rule[ARPortForwardingFieldExternalPort])
}

// units mirrors `_units`: collect the WAN units referenced by a rule list.
// Only int-like units are kept: others never match a list key.
func units(rules []ARPortForwardingRule) map[int]bool {
	out := map[int]bool{}
	for _, rule := range rules {
		if unit, ok := unitKey(get(rule, ARPortForwardingFieldWanUnit, 0)); ok {
			out[unit] = true
		}
	}
	return out
}

// byUnit mirrors `_by_unit`: filter rules belonging to a WAN unit.
func byUnit(rules []ARPortForwardingRule, unit int) []ARPortForwardingRule {
	out := []ARPortForwardingRule{}
	for _, rule := range rules {
		if u, ok := unitKey(get(rule, ARPortForwardingFieldWanUnit, 0)); ok && u == unit {
			out = append(out, rule)
		}
	}
	return out
}

// stateArguments mirrors `_state_arguments`: build the arguments for a
// master toggle.
func stateArguments(a *ARPortForwardingAction) [][2]any {
	if a.State == nil {
		return nil
	}
	state := 0
	if *a.State {
		state = 1
	}
	return [][2]any{{string(nvram.ARNvramTypePortForwardingState), state}}
}

// fetchRules mirrors `_fetch_rules`: read the current rules, or ok=false
// when they cannot be fetched.
func fetchRules(ctx context.Context, c *registry.Context) ([]ARPortForwardingRule, bool, error) {
	if c.FetchData == nil {
		return nil, false, nil
	}
	fetched, err := c.FetchData(ctx, ARPortForwardingSourceUniversal, nil)
	if err != nil {
		return nil, false, err
	}
	if fetched == nil {
		return nil, false, nil
	}
	data, _ := fetched.(map[ARPortForwardingField]any)
	rules, _ := data[ARPortForwardingFieldRules].([]ARPortForwardingRule)
	return append([]ARPortForwardingRule{}, rules...), true, nil
}

func union(sets ...map[int]bool) map[int]bool {
	out := map[int]bool{}
	for _, s := range sets {
		for k := range s {
			out[k] = true
		}
	}
	return out
}

func allComplete(rules []ARPortForwardingRule) bool {
	for _, rule := range rules {
		if !isComplete(rule) {
			return false
		}
	}
	return true
}

// rulesArguments mirrors `_rules_arguments`: build the arguments for an
// add/remove/set of rules (nil when rejected).
func rulesArguments(ctx context.Context, a *ARPortForwardingAction, c *registry.Context) ([][2]any, error) {
	var final []ARPortForwardingRule
	var touched map[int]bool

	switch a.Command {
	case ARPortForwardingCommandSet:
		// Refuse to write a list containing an incomplete rule
		if !allComplete(a.Rules) {
			slog.Debug("SET rejected: an incomplete rule was given")
			return nil, nil
		}
		final = append([]ARPortForwardingRule{}, a.Rules...)
		// Always rewrite the primary list, so an empty set clears it
		touched = union(units(final), map[int]bool{0: true})
	case ARPortForwardingCommandAdd:
		if len(a.Rules) == 0 || !allComplete(a.Rules) {
			slog.Debug("ADD rejected: no rules or an incomplete one given")
			return nil, nil
		}
		current, ok, err := fetchRules(ctx, c)
		if err != nil || !ok {
			return nil, err
		}
		final = append(current, a.Rules...)
		touched = union(units(current), units(final))
	default: // REMOVE
		specs := []ARPortForwardingRule{}
		for _, spec := range a.Rules {
			if len(spec) > 0 {
				specs = append(specs, spec)
			}
		}
		if len(specs) == 0 {
			slog.Debug("REMOVE rejected: no match specs given")
			return nil, nil
		}
		current, ok, err := fetchRules(ctx, c)
		if err != nil || !ok {
			return nil, err
		}
		final = []ARPortForwardingRule{}
		for _, rule := range current {
			matched := false
			for _, spec := range specs {
				if specMatches(spec, rule) {
					matched = true
					break
				}
			}
			if !matched {
				final = append(final, rule)
			}
		}
		// Include emptied units so a full removal still clears them
		touched = union(units(current), units(final))
	}

	// Collapse duplicates: an added rule already present, or stale doubles
	// left in the saved list
	final = dedup(final)

	sorted := make([]int, 0, len(touched))
	for unit := range touched {
		sorted = append(sorted, unit)
	}
	sort.Ints(sorted)

	arguments := [][2]any{}
	for _, unit := range sorted {
		if key, ok := listKeys[unit]; ok {
			arguments = append(arguments, [2]any{key, SerializeRules(byUnit(final, unit))})
		}
	}
	return arguments, nil
}

// runAction mirrors `run_action`: apply the port forwarding action via
// `restart_firewall`.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARPortForwardingAction)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	var arguments [][2]any
	switch act.Command {
	case ARPortForwardingCommandState:
		arguments = stateArguments(act)
	case ARPortForwardingCommandAdd, ARPortForwardingCommandRemove, ARPortForwardingCommandSet:
		var err error
		arguments, err = rulesArguments(ctx, act, c)
		if err != nil {
			return nil, err
		}
	default:
		slog.Debug("Unknown port forwarding command", "command", act.Command)
		return service.ARServiceResult{Success: false}, nil
	}

	if len(arguments) == 0 {
		return service.ARServiceResult{Success: false}, nil
	}

	result, err := service.AsyncRunService(
		ctx,
		c.Read,
		common.ARServiceFirewallRestart,
		arguments,
		common.ARActionModeApply,
		c.FetchRaw,
	)
	if err != nil {
		return nil, err
	}

	// Drop the now-stale cached rules/state so the next read refetches
	if result.Success && c.Expire != nil {
		c.Expire(ctx, ARPortForwardingSourceUniversal)
	}

	return result, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARPortForwardingAction",
		Run:  runAction,
	})
}
