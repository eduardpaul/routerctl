// Package source mirrors asusrouter/modules/aimesh/source.py (split out of
// package aimesh to break the identity import cycle).
package source

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// ARAiMeshSource is the AiMesh topology data source.
type ARAiMeshSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARAiMeshSource) SourceName() string { return "ARAiMeshSource" }

// ARAiMeshSourceUniversal is the universal instance - preferred.
var ARAiMeshSourceUniversal = &ARAiMeshSource{}

// fetchState fetches the raw onboarding data.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	if c.Identity == nil || !support.SupportAvailable(c.Identity.Support(), support.ARSupportTypeAimesh) {
		return map[string]any{}, nil
	}

	response, err := c.Read(ctx, endpoint.AREndpointFetchOnboarding, "")
	if err != nil {
		return nil, err
	}

	if m, ok := response.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// translateState translates raw onboarding data into the AiMesh topology.
func translateState(raw any, tc *registry.TranslateContext) any {
	data, ok := raw.(map[string]any)
	if !ok || len(data) == 0 {
		return aimesh.NewARAiMeshTopology()
	}

	var onboarding aimesh.RawDict
	if m, ok := readers.ReadJSSection(data, "get_onboardinglist").(map[string]any); ok {
		onboarding = m
	}
	status := aimesh.TranslateOnboardingStatus(readers.ReadJSSection(data, "get_onboardingstatus"))
	return aimesh.TranslateTopology(
		readers.ReadJSSection(data, "get_cfg_clientlist"),
		&status,
		onboarding,
	)
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARAiMeshSource",
		Default:   func() source.DataSource { return &ARAiMeshSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
