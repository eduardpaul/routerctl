package common

import (
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Coded prefixes mapped to a display-name template
var vendorPrefixes = [][2]string{
	{"android-dhcp-", "Android {}"},
}

// Exact coded values mapped to their proper name
var vendorAliases = map[string]string{
	"MSFT 5.0": "Microsoft Corporation",
}

// FormatVendor formats a coded vendor value into a readable name.
func FormatVendor(vendor *string) *string {
	if vendor == nil || *vendor == "" {
		return vendor
	}
	v := *vendor

	for _, pt := range vendorPrefixes {
		prefix, template := pt[0], pt[1]
		if strings.HasPrefix(v, prefix) {
			out := converters.PyStrip(strings.Replace(template, "{}", v[len(prefix):], 1))
			return &out
		}
	}

	out := v
	if alias, ok := vendorAliases[v]; ok {
		out = alias
	}
	out = converters.PyStrip(out)
	return &out
}
