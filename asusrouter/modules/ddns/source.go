// Package ddns mirrors asusrouter/modules/ddns/.
package ddns

import (
	"context"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Numeric status codes matched as substrings, e.g. `register,230`
// The values are distinct, so match order does not matter
var statusCodes = func() []ARDdnsStatus {
	var out []ARDdnsStatus
	for _, status := range ARDdnsStatusMembers {
		digits := strings.TrimLeft(string(status), "-")
		if digits != "" && isDigits(digits) {
			out = append(out, status)
		}
	}
	return out
}()

// isDigits mirrors Python `str.isdigit` for ASCII text.
func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ARDdnsSource mirrors `ARDdnsSource`: the AsusRouter DDNS data source.
type ARDdnsSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARDdnsSource) SourceName() string { return "ARDdnsSource" }

// ARDdnsSourceUniversal is the universal instance - preferred.
var ARDdnsSourceUniversal = &ARDdnsSource{}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch the DDNS configuration through the NVRAM module
	return nvramsource.AsyncFetchValues(ctx, c, DdnsRequest, nil)
}

// ReadStatus mirrors `read_status`: the DDNS status from a raw return
// code.
func ReadStatus(raw any) ARDdnsStatus {
	value := converters.RawToStr(raw)
	if value == nil {
		return ARDdnsStatusNone
	}

	// Text statuses are full-string values
	if status := ARDdnsStatusFromValue(*value); status != ARDdnsStatusUnknown {
		return status
	}

	// Numeric codes can be embedded in a longer string
	for _, code := range statusCodes {
		if strings.Contains(*value, string(code)) {
			return code
		}
	}

	return ARDdnsStatusUnknown
}

func readServer(raw any) any {
	// Read the DDNS server, nil when not selected
	value := converters.RawToStr(raw)
	if value == nil {
		return nil
	}
	return ARDdnsServerFromValue(*value)
}

func readState(raw any) any {
	// Read the master DDNS state
	b := converters.RawToBool(raw)
	return b != nil && *b
}

func readStatus(raw any) any { return ReadStatus(raw) }

// Translation map: nvram key -> field, converter
var translation = []struct {
	key     nvram.ARNvramType
	field   ARDdnsField
	convert func(any) any
}{
	{nvram.ARNvramTypeDdnsHostname, ARDdnsFieldHostname, converters.AsAny(identifiers.HostnameFromValueSafe)},
	{nvram.ARNvramTypeDdnsHostnameOld, ARDdnsFieldHostnameOld, converters.AsAny(identifiers.HostnameFromValueSafe)},
	{nvram.ARNvramTypeDdnsIpAddress, ARDdnsFieldIpAddress, converters.AsAny(identifiers.IpAddressFromValueSafe)},
	{nvram.ARNvramTypeDdnsIpv6Update, ARDdnsFieldIpv6Update, converters.AsAny(converters.RawToBool)},
	{nvram.ARNvramTypeDdnsPassword, ARDdnsFieldPassword, converters.AsAny(identifiers.PasswordFromValueSafe)},
	{nvram.ARNvramTypeDdnsRefreshInterval, ARDdnsFieldRefreshInterval, converters.AsAny(converters.RawToInt)},
	{nvram.ARNvramTypeDdnsRegularCheck, ARDdnsFieldVerification, converters.AsAny(converters.RawToBool)},
	{nvram.ARNvramTypeDdnsRegularPeriod, ARDdnsFieldVerificationPeriod, converters.AsAny(converters.RawToInt)},
	{nvram.ARNvramTypeDdnsReplaceStatus, ARDdnsFieldReplaceStatus, converters.AsAny(converters.RawToBool)},
	{nvram.ARNvramTypeDdnsReturnCodeChk, ARDdnsFieldStatus, readStatus},
	{nvram.ARNvramTypeDdnsServer, ARDdnsFieldServer, readServer},
	{nvram.ARNvramTypeDdnsState, ARDdnsFieldState, readState},
	{nvram.ARNvramTypeDdnsTokenState, ARDdnsFieldTokenState, converters.AsAny(converters.RawToBool)},
	{nvram.ARNvramTypeDdnsUpdated, ARDdnsFieldUpdated, converters.AsAny(converters.RawToBool)},
	{nvram.ARNvramTypeDdnsUsername, ARDdnsFieldUsername, converters.AsAny(converters.RawToStr)},
	{nvram.ARNvramTypeDdnsWanUnit, ARDdnsFieldWanUnit, converters.AsAny(converters.RawToInt)},
	{nvram.ARNvramTypeDdnsWildcard, ARDdnsFieldWildcard, converters.AsAny(converters.RawToBool)},
}

// DdnsRequest is the full NVRAM request for the DDNS state, built once.
var DdnsRequest = func() []nvram.ARNvramType {
	out := make([]nvram.ARNvramType, len(translation))
	for i, row := range translation {
		out[i] = row.key
	}
	return out
}()

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate raw DDNS nvram into a structured dict
	data, ok := raw.(map[source.ID]any)
	if !ok || len(data) == 0 {
		return ARDdnsConfig{}
	}

	out := ARDdnsConfig{}
	for _, row := range translation {
		out[row.field] = row.convert(data[source.IDOf(row.key)])
	}
	return out
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARDdnsSource",
		Default:   func() source.DataSource { return &ARDdnsSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
