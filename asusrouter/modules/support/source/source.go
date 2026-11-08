// Package source mirrors the data-source part of
// asusrouter/modules/support/__init__.py (fetch, translation table,
// registration) plus the per-feature translators (`ai.py`, `wifi.py`,
// ...). They are split out of package support because they import
// modules that register sources, which the device identity cannot depend
// on (see PARITY.md, split packages).
package source

import (
	"context"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func asAny[T any](f func(map[string]any) T) func(map[string]any) any {
	return func(data map[string]any) any { return f(data) }
}

var translationTable = map[support.ARSupportType]func(map[string]any) any{
	support.ARSupportTypeAi:                          asAny(translateAi),
	support.ARSupportTypeAiCapabilities:              asAny(translateAiCapabilities),
	support.ARSupportTypeAimesh:                      asAny(translateAimesh),
	support.ARSupportTypeAimeshCapabilities:          asAny(translateAimeshCapabilities),
	support.ARSupportTypeAura:                        asAny(translateAura),
	support.ARSupportTypeAuraCapabilities:            asAny(translateAuraCapabilities),
	support.ARSupportTypeConnections:                 asAny(translateConnection),
	support.ARSupportTypeCredentialsCapabilities:     asAny(translateCredentialsCapabilities),
	support.ARSupportTypeDeviceMode:                  asAny(translateDeviceMode),
	support.ARSupportTypeDsl:                         asAny(translateDsl),
	support.ARSupportTypeFirmware:                    asAny(translateFirmware),
	support.ARSupportTypeFirmwareCapabilities:        asAny(translateFirmwareCapabilities),
	support.ARSupportTypeFtp:                         asAny(translateFtp),
	support.ARSupportTypeFtpCapabilities:             asAny(translateFtpCapabilities),
	support.ARSupportTypeLanCapabilities:             asAny(translateLanCapabilities),
	support.ARSupportTypeParentalControl:             asAny(translateParentalControl),
	support.ARSupportTypeParentalControlCapabilities: asAny(translateParentalControlCapabilities),
	support.ARSupportTypePlatform:                    asAny(translatePlatform),
	support.ARSupportTypeSdn:                         asAny(translateSdn),
	support.ARSupportTypeSdnCapabilities:             asAny(translateSdnCapabilities),
	support.ARSupportTypeSpeedtest:                   asAny(translateSpeedtest),
	support.ARSupportTypeSpeedtestCapabilities:       asAny(translateSpeedtestCapabilities),
	support.ARSupportTypeUsb:                         asAny(translateUsb),
	support.ARSupportTypeUsbCapabilities:             asAny(translateUsbCapabilities),
	support.ARSupportTypeVpn:                         asAny(translateVpn),
	support.ARSupportTypeVpnCapabilities:             asAny(translateVpnCapabilities),
	support.ARSupportTypeWan:                         asAny(translateWan),
	support.ARSupportTypeWanCapabilities:             asAny(translateWanCapabilities),
	support.ARSupportTypeWifi:                        asAny(translateWifi),
	support.ARSupportTypeWifiCapabilities:            asAny(translateWifiCapabilities),
}

// fetchRcSupport fetches `rc_support` nvram as a {token: True} map
// (old-firmware path).
func fetchRcSupport(ctx context.Context, c *registry.Context) (map[string]any, error) {
	key := string(nvram.ARNvramTypeRcSupport)
	response, err := c.Read(ctx, endpoint.AREndpointFetchData, endpoint.HookRequest(nvram.ARNvramTypeRcSupport))
	if err != nil {
		return nil, err
	}
	var tokens any
	if m, ok := response.(map[string]any); ok {
		tokens = m[key]
	}
	if s, ok := tokens.(string); ok && s != "" {
		out := map[string]any{}
		for _, token := range converters.PySplit(s) {
			out[token] = true
		}
		return out, nil
	}

	return map[string]any{}, nil
}

// fetchState fetches the support data state.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	response, err := c.Read(ctx, endpoint.AREndpointFetchData, endpoint.HookRequest(endpoint.ARHookUiSupport))
	if err != nil {
		return nil, err
	}
	if m, ok := response.(map[string]any); ok {
		if uiSupport, ok := m[string(endpoint.ARHookUiSupport)].(map[string]any); ok && len(uiSupport) > 0 {
			return uiSupport, nil
		}
	}

	// Old firmware returns an empty `get_ui_support` - fall back to rc_support
	return fetchRcSupport(ctx, c)
}

// TranslateState translates the support data to a simple format.
func TranslateState(data map[string]any) map[support.ARSupportType]any {
	result := make(map[support.ARSupportType]any, len(translationTable))
	for supportType, interpreter := range translationTable {
		result[supportType] = interpreter(data)
	}
	return result
}

func translateState(raw any, tc *registry.TranslateContext) any {
	data, _ := raw.(map[string]any)
	return TranslateState(data)
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARSupportSource",
		Default:   func() source.DataSource { return &support.ARSupportSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
