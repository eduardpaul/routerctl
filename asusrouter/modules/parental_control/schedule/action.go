package schedule

import (
	"context"
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	pc "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// (ARParentalControlRule is declared in source.go: Go keeps one
// declaration per package.)

// DefaultMaxRules is the rule count the device falls back to when
// `MaxRule_parentctrl` is absent.
const DefaultMaxRules = 16

// DefaultMaxEntries is the total schedule windows the device falls back to
// for `MaxRule_PC_DAYTIME`.
const DefaultMaxEntries = 128

// ScheduleModeMinVersion is the minimum `PC_SCHED_V3` support for the
// online (allow-only) schedule mode.
const ScheduleModeMinVersion = 3

// ARParentalControlAction mirrors `ARParentalControlAction`: mutate
// parental control rules or a master toggle.
type ARParentalControlAction struct {
	action.Base

	Command pc.ARParentalControlCommand
	State   *bool
	// ADD: rules to append. SET: the full replacement list. Both require
	// complete rules. REMOVE: match specs - a current rule is dropped when a
	// spec matches it on every field the spec sets, so a full read rule is an
	// exact match and a partial one (e.g. `{MAC: ...}`) is a wildcard
	Rules []ARParentalControlRule
}

// ActionName returns the Python class name.
func (*ARParentalControlAction) ActionName() string { return "ARParentalControlAction" }

func norm(field pc.ARParentalControlField, value any) string {
	// Normalize a rule field to a comparable string
	switch field {
	case pc.ARParentalControlFieldMac:
		if mac := identifiers.MacAddressFromValueSafe(value); mac != nil {
			return mac.String()
		}
		return converters.PyStr(value)
	case pc.ARParentalControlFieldType:
		return converters.PyStr(int(pc.ARParentalControlTypeFromValue(value)))
	}
	if value == nil {
		return ""
	}
	return converters.PyStr(value)
}

func specMatches(spec, rule ARParentalControlRule) bool {
	// Whether a rule matches a spec on every field the spec sets
	if len(spec) == 0 {
		// An empty spec must not match everything
		return false
	}
	for field, value := range spec {
		if norm(field, value) != norm(field, rule[field]) {
			return false
		}
	}
	return true
}

func dedupKey(rule ARParentalControlRule) string {
	// Device identity of a rule: one rule per MAC, mirroring the web UI
	return norm(pc.ARParentalControlFieldMac, rule[pc.ARParentalControlFieldMac])
}

func dedup(rules []ARParentalControlRule) []ARParentalControlRule {
	// Drop duplicate rules, keeping the first occurrence of each MAC
	seen := map[string]bool{}
	unique := []ARParentalControlRule{}
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

func isComplete(rule ARParentalControlRule) bool {
	// Whether a rule carries the fields the device needs to store it
	if identifiers.MacAddressFromValueSafe(rule[pc.ARParentalControlFieldMac]) == nil {
		return false
	}
	return pc.ARParentalControlTypeFromValue(rule[pc.ARParentalControlFieldType]) != pc.ARParentalControlTypeUnknown
}

func toggleArguments(a *ARParentalControlAction) [][2]any {
	// Build the arguments for the rules-engine master toggle
	if a.State == nil {
		return nil
	}
	state := 0
	if *a.State {
		state = 1
	}
	return [][2]any{{string(nvram.ARNvramTypeParentalControlState), state}}
}

func capability(identity *device.ARDeviceIdentity, key pc.ARParentalControlCapability) *int {
	if identity == nil {
		return nil
	}
	return converters.RawToInt(support.SupportCapability(
		identity.Support(),
		support.ARSupportTypeParentalControlCapabilities,
		key,
	))
}

func maxRules(identity *device.ARDeviceIdentity) int {
	// Resolve the device's rule limit, or the firmware default
	if value := capability(identity, pc.ARParentalControlCapabilityMaxRules); value != nil && *value != 0 {
		return *value
	}
	return DefaultMaxRules
}

func maxEntries(identity *device.ARDeviceIdentity) int {
	// Resolve the device's total schedule-window limit, or the default
	if value := capability(identity, pc.ARParentalControlCapabilityMaxEntries); value != nil && *value != 0 {
		return *value
	}
	return DefaultMaxEntries
}

func onlineSupported(identity *device.ARDeviceIdentity) bool {
	// Whether the device accepts the online (allow-only) schedule mode
	version := capability(identity, pc.ARParentalControlCapabilitySchedVersion)
	return version != nil && *version >= ScheduleModeMinVersion
}

func modesSupported(rules []ARParentalControlRule, identity *device.ARDeviceIdentity) bool {
	// Whether every online-mode rule is allowed on this device
	if onlineSupported(identity) {
		return true
	}
	for _, rule := range rules {
		if RuleScheduleMode(rule) == pc.ARParentalControlScheduleModeOnline {
			return false
		}
	}
	return true
}

func fetchRules(ctx context.Context, c *registry.Context) ([]ARParentalControlRule, error) {
	// Read the current rules, or nil when they cannot be fetched
	if c.FetchData == nil {
		return nil, nil
	}
	fetched, err := c.FetchData(ctx, ARParentalControlSourceUniversal, nil)
	if err != nil {
		return nil, err
	}
	// Python: `fetched` is {source: content}; no fresh content (nil) or a
	// non-dict is unusable, an empty (nil) dict means no rules
	data, ok := fetched.(map[pc.ARParentalControlField]any)
	if !ok {
		return nil, nil
	}
	rules, _ := data[pc.ARParentalControlFieldRules].([]ARParentalControlRule)
	return append([]ARParentalControlRule{}, rules...), nil
}

func resolveSet(a *ARParentalControlAction) []ARParentalControlRule {
	// Return the SET replacement list, or nil if a rule is incomplete
	for _, rule := range a.Rules {
		if !isComplete(rule) {
			slog.Debug("SET rejected: an incomplete rule was given")
			return nil
		}
	}
	return append([]ARParentalControlRule{}, a.Rules...)
}

func resolveAdd(ctx context.Context, a *ARParentalControlAction, c *registry.Context) ([]ARParentalControlRule, error) {
	// Return current rules with the new ones prepended, or nil to reject
	complete := len(a.Rules) > 0
	for _, rule := range a.Rules {
		if !isComplete(rule) {
			complete = false
		}
	}
	if !complete {
		slog.Debug("ADD rejected: no rules or an incomplete one given")
		return nil, nil
	}
	current, err := fetchRules(ctx, c)
	if err != nil || current == nil {
		return nil, err
	}
	// An added MAC replaces the existing rule for that MAC
	return append(append([]ARParentalControlRule{}, a.Rules...), current...), nil
}

func resolveRemove(ctx context.Context, a *ARParentalControlAction, c *registry.Context) ([]ARParentalControlRule, error) {
	// Return current rules minus those matching a spec, or nil to reject
	var specs []ARParentalControlRule
	for _, spec := range a.Rules {
		if len(spec) > 0 {
			specs = append(specs, spec)
		}
	}
	if len(specs) == 0 {
		slog.Debug("REMOVE rejected: no match specs given")
		return nil, nil
	}
	current, err := fetchRules(ctx, c)
	if err != nil || current == nil {
		return nil, err
	}
	out := []ARParentalControlRule{}
	for _, rule := range current {
		matched := false
		for _, spec := range specs {
			if specMatches(spec, rule) {
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, rule)
		}
	}
	return out, nil
}

func resolveRules(ctx context.Context, a *ARParentalControlAction, c *registry.Context) ([]ARParentalControlRule, error) {
	// Resolve the final rule list for an add/remove/set, or nil to reject
	switch a.Command {
	case pc.ARParentalControlCommandSet:
		return resolveSet(a), nil
	case pc.ARParentalControlCommandAdd:
		return resolveAdd(ctx, a, c)
	}
	return resolveRemove(ctx, a, c)
}

func rulesArguments(ctx context.Context, a *ARParentalControlAction, c *registry.Context, identity *device.ARDeviceIdentity) ([][2]any, error) {
	// Build the arguments for an add/remove/set of rules
	resolved, err := resolveRules(ctx, a, c)
	if err != nil || resolved == nil {
		return nil, err
	}

	// Collapse duplicates, keeping the first rule per MAC
	final := dedup(resolved)

	// REMOVE only shrinks the list and keeps existing rules as-is
	if a.Command == pc.ARParentalControlCommandRemove {
		return SerializeRules(final), nil
	}

	// ADD/SET introduce rules: reject an online-mode rule the device rejects
	if !modesSupported(a.Rules, identity) {
		slog.Debug("Rejected: online schedule mode is unsupported", "command", a.Command)
		return nil, nil
	}

	// The device rejects a list longer than its rule limit
	if limit := maxRules(identity); len(final) > limit {
		slog.Debug("Rejected: rules exceed the limit", "command", a.Command, "rules", len(final), "limit", limit)
		return nil, nil
	}

	// The device also caps the total schedule windows across every rule
	entries := 0
	for _, rule := range final {
		entries += RuleEntryCount(rule)
	}
	if entryLimit := maxEntries(identity); entries > entryLimit {
		slog.Debug("Rejected: schedule windows exceed the limit", "command", a.Command, "entries", entries, "limit", entryLimit)
		return nil, nil
	}

	return SerializeRules(final), nil
}

// RunAction mirrors `run_action`: apply the parental control action via
// `restart_firewall`.
func RunAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act := a.(*ARParentalControlAction)
	identity := c.Identity

	var arguments [][2]any
	switch act.Command {
	case pc.ARParentalControlCommandState:
		arguments = toggleArguments(act)
	case pc.ARParentalControlCommandAdd, pc.ARParentalControlCommandRemove, pc.ARParentalControlCommandSet:
		args, err := rulesArguments(ctx, act, c, identity)
		if err != nil {
			return nil, err
		}
		arguments = args
	default:
		slog.Debug("Unknown parental control command", "command", act.Command)
		return service.ARServiceResult{Success: false}, nil
	}

	if len(arguments) == 0 {
		return service.ARServiceResult{Success: false}, nil
	}

	result, err := service.AsyncRunService(ctx, c.Read, common.ARServiceFirewallRestart, arguments, common.ARActionModeApply, c.FetchRaw)
	if err != nil {
		return nil, err
	}

	// Drop the now-stale cached rules/state so the next read refetches
	if result.Success && c.Expire != nil {
		c.Expire(ctx, ARParentalControlSourceUniversal)
		nvramsource.AsyncExpireValues(ctx, c.Expire, PcRequest)
	}

	return result, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARParentalControlAction",
		Run:  RunAction,
	})
}
