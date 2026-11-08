// Package targets mirrors asusrouter/modules/ping/targets/.
package targets

import (
	"context"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// Data model

// ARPingTarget mirrors `ARPingTarget`: a DNS ping target.
type ARPingTarget struct {
	Name string                `json:"name"`
	IP   identifiers.IpAddress `json:"ip"`
}

// ARPingTargetInput is a target, or something coercible into one
// (ARPingTarget, identifiers.IpAddress or string).
type ARPingTargetInput = any

// Source

// ARPingTargetsSource mirrors `ARPingTargetsSource`: the AsusRouter ping
// targets data source.
type ARPingTargetsSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARPingTargetsSource) SourceName() string { return "ARPingTargetsSource" }

// ARPingTargetsSourceUniversal is the universal instance - preferred.
var ARPingTargetsSourceUniversal = &ARPingTargetsSource{}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch the raw dns_ping_list value
	value, err := nvramsource.AsyncGetValue(ctx, c, nvram.ARNvramTypeDnsPingList, nil)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	return value, nil
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate the raw dns_ping_list value into targets
	data, ok := raw.(string)
	if !ok {
		return []ARPingTarget{}
	}
	return parseTargets(data)
}

func parseTargets(raw string) []ARPingTarget {
	// Parse a `<name>ip` delimited dns_ping_list value into targets
	targets := []ARPingTarget{}
	for _, entry := range readers.SplitRows(raw) {
		if entry == "" {
			continue
		}
		name, ipRaw, _ := strings.Cut(entry, ">")
		if ip := identifiers.IpAddressFromValueSafe(ipRaw); ip != nil {
			targets = append(targets, ARPingTarget{Name: name, IP: *ip})
		}
	}
	return targets
}

// Registration

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARPingTargetsSource",
		Default:   func() source.DataSource { return &ARPingTargetsSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
