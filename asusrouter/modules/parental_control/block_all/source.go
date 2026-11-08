// Package blockall mirrors asusrouter/modules/parental_control/block_all/.
package blockall

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// KeyBlockAll is `MULTIFILTER_BLOCK_ALL`: it cuts internet for every
// device at once.
const KeyBlockAll = nvram.ARNvramTypeParentalControlBlockAll

// ARBlockAllSource mirrors `ARBlockAllSource`: the AsusRouter
// block-all-devices data source.
type ARBlockAllSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARBlockAllSource) SourceName() string { return "ARBlockAllSource" }

// ARBlockAllSourceUniversal is the universal instance - preferred.
var ARBlockAllSourceUniversal = &ARBlockAllSource{}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch the block-all switch through the NVRAM module
	return nvramsource.AsyncFetchValues(ctx, c, KeyBlockAll, nil)
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate the raw block-all nvram into a boolean
	data, ok := raw.(map[source.ID]any)
	if !ok {
		return false
	}
	b := converters.RawToBool(data[source.IDOf(KeyBlockAll)])
	return b != nil && *b
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARBlockAllSource",
		Default:   func() source.DataSource { return &ARBlockAllSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
