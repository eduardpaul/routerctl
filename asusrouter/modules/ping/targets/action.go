package targets

import (
	"context"
	"log/slog"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Default name for a target added by bare IP; names need not be unique
const defaultTargetName = "target"

func normalize(target ARPingTargetInput) *ARPingTarget {
	// Coerce a target, IP, or IP string into a target; drop if invalid.
	//
	// The IP is always coerced to `IpAddress` - a target constructed with a
	// string IP would otherwise break IP-keyed dedup and comparison.
	name, value := defaultTargetName, target
	switch t := target.(type) {
	case ARPingTarget:
		name, value = t.Name, t.IP
	case *ARPingTarget:
		if t == nil {
			return nil
		}
		name, value = t.Name, t.IP
	}

	ip := identifiers.IpAddressFromValueSafe(value)
	if ip == nil {
		return nil
	}
	return &ARPingTarget{Name: name, IP: *ip}
}

// ARPingTargetsAction mirrors `ARPingTargetsAction`: add, remove, or clean
// the DNS ping target list.
type ARPingTargetsAction struct {
	action.Base

	Op      action.ARActionType
	Targets []ARPingTarget
}

// NewARPingTargetsAction initializes the action with an operation and
// optional target(s): nil, a single ARPingTargetInput or a slice of them.
func NewARPingTargetsAction(op action.ARActionType, targets any) *ARPingTargetsAction {
	// Accept a single target or a list; a bare str must not be iterated
	var provided []ARPingTargetInput
	switch t := targets.(type) {
	case nil:
	case []ARPingTargetInput:
		provided = t
	case []ARPingTarget:
		for _, item := range t {
			provided = append(provided, item)
		}
	case []identifiers.IpAddress:
		for _, item := range t {
			provided = append(provided, item)
		}
	case []string:
		for _, item := range t {
			provided = append(provided, item)
		}
	default:
		provided = []ARPingTargetInput{t}
	}

	a := &ARPingTargetsAction{Op: op, Targets: []ARPingTarget{}}
	for _, target := range provided {
		if normalized := normalize(target); normalized != nil {
			a.Targets = append(a.Targets, *normalized)
		}
	}
	return a
}

// ActionName returns the Python class name.
func (*ARPingTargetsAction) ActionName() string { return "ARPingTargetsAction" }

func encodeTargets(targets []ARPingTarget) string {
	// Encode targets into a `<name>ip` delimited dns_ping_list value
	var b strings.Builder
	for _, target := range targets {
		b.WriteString("<" + target.Name + ">" + target.IP.String())
	}
	return b.String()
}

// orderedByIP is an insertion-ordered map of targets keyed by IP (Python
// dict semantics: an update keeps the first position).
type orderedByIP struct {
	order []identifiers.IpAddress
	items map[identifiers.IpAddress]ARPingTarget
}

func (o *orderedByIP) set(target ARPingTarget) {
	if o.items == nil {
		o.items = map[identifiers.IpAddress]ARPingTarget{}
	}
	if _, ok := o.items[target.IP]; !ok {
		o.order = append(o.order, target.IP)
	}
	o.items[target.IP] = target
}

func (o *orderedByIP) values() []ARPingTarget {
	out := make([]ARPingTarget, 0, len(o.order))
	for _, ip := range o.order {
		out = append(out, o.items[ip])
	}
	return out
}

func add(current, newTargets []ARPingTarget) []ARPingTarget {
	// Merge by IP: update the name on a match, append new IPs.
	//
	// The result is unique by IP, collapsing any pre-existing duplicates.
	var merged orderedByIP
	for _, target := range current {
		merged.set(target)
	}
	for _, target := range newTargets {
		merged.set(target)
	}
	return merged.values()
}

func remove(current, drop []ARPingTarget) []ARPingTarget {
	// Drop targets by IP match, ignoring the name.
	//
	// The result is unique by IP, collapsing any pre-existing duplicates.
	dropped := map[identifiers.IpAddress]bool{}
	for _, target := range drop {
		dropped[target.IP] = true
	}
	var merged orderedByIP
	for _, target := range current {
		if !dropped[target.IP] {
			merged.set(target)
		}
	}
	return merged.values()
}

func equalTargets(a, b []ARPingTarget) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func asyncCurrent(ctx context.Context, c *registry.Context) ([]ARPingTarget, error) {
	// Fetch the current target list, bypassing the cache
	raw, err := nvramsource.AsyncGetValue(ctx, c, nvram.ARNvramTypeDnsPingList, registry.Kwargs{"force": true})
	if err != nil {
		return nil, err
	}
	if s, ok := raw.(string); ok {
		return parseTargets(s), nil
	}
	return []ARPingTarget{}, nil
}

// RunAction mirrors `run_action`: apply an add/remove/clean operation to
// the ping target list.
func RunAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act := a.(*ARPingTargetsAction)

	var targets []ARPingTarget
	switch act.Op {
	case action.ARActionTypeClean:
		targets = []ARPingTarget{}
	case action.ARActionTypeAdd, action.ARActionTypeRemove:
		if len(act.Targets) == 0 {
			slog.Debug("No compatible target provided; nothing to apply")
			return service.ARServiceResult{Success: false}, nil
		}
		if c.FetchDataMulti == nil {
			return service.ARServiceResult{Success: false}, nil
		}
		current, err := asyncCurrent(ctx, c)
		if err != nil {
			return nil, err
		}
		if act.Op == action.ARActionTypeAdd {
			targets = add(current, act.Targets)
		} else {
			targets = remove(current, act.Targets)
		}
		if equalTargets(targets, current) {
			slog.Debug("Target list unchanged; nothing to apply")
			return service.ARServiceResult{Success: true}, nil
		}
	default:
		return service.ARServiceResult{Success: false}, nil
	}

	request := endpoint.BuildPushRequest("", [][2]any{
		{string(nvram.ARNvramTypeDnsPingList), encodeTargets(targets)},
	})
	data, err := c.Read(ctx, endpoint.AREndpointPushData, request)
	if err != nil {
		return nil, err
	}
	m, ok := data.(map[string]any)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}
	modify := converters.RawToBool(m[common.ModifyKey])
	return service.ARServiceResult{Success: modify != nil && *modify}, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARPingTargetsAction",
		Run:  RunAction,
	})
}
