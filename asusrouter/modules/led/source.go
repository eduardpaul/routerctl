// Package led mirrors asusrouter/modules/led/.
package led

// LED data source for AsusRouter.

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	aimeshsource "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// ARLedSource mirrors `ARLedSource`: AsusRouter LED data source.
type ARLedSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARLedSource) SourceName() string { return "ARLedSource" }

// ARLedSourceUniversal is the universal instance - preferred.
var ARLedSourceUniversal = &ARLedSource{}

// fetchState mirrors `fetch_state`: fetch the LED state through the NVRAM
// module and AiMesh. The result is keyed by NVRAM item ID.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	values, err := nvramsource.AsyncFetchValues(ctx, c, LedRequest, nil)
	if err != nil {
		return nil, err
	}

	// The AiMesh page switches the LED in the mesh config only, leaving
	// nvram on the previous state
	led, err := asyncGetAimeshLed(ctx, c.FetchData, c.Identity)
	if err != nil {
		return nil, err
	}
	if led != nil {
		merged := make(map[source.ID]any, len(values)+1)
		for k, v := range values {
			merged[k] = v
		}
		merged[source.IDOf(nvram.ARNvramTypeLed)] = *led
		values = merged
	}

	return values, nil
}

// asyncGetAimeshLed gets the LED state of the device from its AiMesh
// config.
func asyncGetAimeshLed(ctx context.Context, fetchData registry.FetchDataFunc, identity *device.ARDeviceIdentity) (*bool, error) {
	if fetchData == nil || identity == nil {
		return nil, nil
	}
	fetched, err := fetchData(ctx, aimeshsource.ARAiMeshSourceUniversal, nil)
	if err != nil {
		return nil, err
	}
	topology, ok := fetched.(*aimesh.ARAiMeshTopology)
	if !ok || topology == nil {
		return nil, nil
	}
	mac := identity.Mac()
	if mac == nil {
		return nil, nil
	}
	node := topology.Get(*mac)
	if node == nil {
		return nil, nil
	}
	return node.Led, nil
}

// One translation: nvram key -> field, converter
type translation struct {
	key     nvram.ARNvramType
	field   ARLedField
	convert func(any) any
}

// Translation map: nvram key -> field, converter
var translations = []translation{
	{nvram.ARNvramTypeLed, ARLedFieldState, converters.AsAny(converters.RawToBool)},
}

// LedRequest mirrors `LED_REQUEST`.
// Full NVRAM request for the LED state, built once
var LedRequest = func() []nvram.ARNvramType {
	out := make([]nvram.ARNvramType, 0, len(translations))
	for _, t := range translations {
		out = append(out, t.key)
	}
	return out
}()

// translateState mirrors `translate_state`: translate raw LED nvram into
// a structured dict.
func translateState(raw any, _ *registry.TranslateContext) any {
	data, _ := raw.(map[source.ID]any)
	if len(data) == 0 {
		return map[ARLedField]any{}
	}

	result := make(map[ARLedField]any, len(translations))
	for _, t := range translations {
		result[t.field] = t.convert(data[source.IDOf(t.key)])
	}
	return result
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARLedSource",
		Default:   func() source.DataSource { return &ARLedSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
