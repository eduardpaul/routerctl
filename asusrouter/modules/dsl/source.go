// Package dsl mirrors asusrouter/modules/dsl/ (`__init__.py` only
// re-exports source.py).
package dsl

import (
	"context"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/units"
)

// rateKey is one `(nvram key, metric)` pair of `_RATE_KEYS`.
type rateKey struct {
	key    nvram.ARNvramType
	metric common.ARMetricType
}

// The nvram key for each data rate direction and its target metric
var rateKeys = []rateKey{
	{nvram.ARNvramTypeDslDatarateDown, common.ARMetricTypeDownloadSpeed},
	{nvram.ARNvramTypeDslDatarateUp, common.ARMetricTypeUploadSpeed},
}

// Full NVRAM request for the DSL rates, built once
var dslRequest = func() []nvram.ARNvramType {
	out := make([]nvram.ARNvramType, len(rateKeys))
	for i, rk := range rateKeys {
		out[i] = rk.key
	}
	return out
}()

// readRate mirrors `_read_rate`: read a DSL data rate (`<n> Kbps`) as a
// rate in bits per second.
func readRate(value any) *float64 {
	if value == nil {
		return nil
	}
	// The value carries a unit suffix (e.g. `12345 Kbps`); keep the number
	number := converters.RawToInt(strings.Split(converters.PyStr(value), " ")[0])
	if number == nil {
		return nil
	}
	// The unit is a constant member of the table, so this cannot fail
	rate, _ := units.DataRateUnitConverter.ConvertToBase(float64(*number), units.UnitOfDataRateKilobitPerSecond)
	return &rate
}

// ARDSLSource mirrors `ARDSLSource`: DSL data source for the connected
// router.
type ARDSLSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARDSLSource) SourceName() string { return "ARDSLSource" }

// ARDSLSourceUniversal is the universal instance - preferred
var ARDSLSourceUniversal = &ARDSLSource{}

// fetchState mirrors `fetch_state`: fetch the DSL data rates through the
// NVRAM module.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	var identity *device.ARDeviceIdentity
	if c != nil {
		identity = c.Identity
	}
	if identity == nil || !support.SupportAvailable(identity.Support(), support.ARSupportTypeDsl) {
		return map[source.ID]any{}, nil
	}

	return nvramsource.AsyncFetchValues(ctx, c, dslRequest, nil)
}

// translateState mirrors `translate_state`: translate raw DSL data into the
// flat data rate metrics.
func translateState(raw any, _ *registry.TranslateContext) any {
	data, ok := raw.(map[source.ID]any)
	if !ok {
		return map[common.ARMetricType]any{}
	}

	result := map[common.ARMetricType]any{}
	for _, rk := range rateKeys {
		if rate := readRate(data[source.IDOf(rk.key)]); rate != nil {
			result[rk.metric] = *rate
		}
	}

	return result
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARDSLSource",
		Default:   func() source.DataSource { return &ARDSLSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
