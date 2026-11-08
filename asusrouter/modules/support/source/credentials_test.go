package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/credentials"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

// Only advertised login capabilities are reported.
func TestTranslateCredentialsCapabilities(t *testing.T) {
	type caps = map[credentials.ARCredentialsCapability]any
	check(t, "translateCredentialsCapabilities", translateCredentialsCapabilities, []struct {
		data     d
		expected caps
	}{
		// Nothing advertised
		{d{}, caps{}},
		// Modern firmware reporting the full set
		{
			d{
				k(support.ARSupportValueChpass):                1,
				k(support.ARSupportValueSecureDefault):         1,
				k(support.ARSupportValueHttpUsernameMaxLength): "32",
				k(support.ARSupportValueHttpPasswordMaxLength): "16",
			},
			caps{
				credentials.ARCredentialsCapabilityChpass:            true,
				credentials.ARCredentialsCapabilitySecureDefault:     true,
				credentials.ARCredentialsCapabilityUsernameMaxLength: 32,
				credentials.ARCredentialsCapabilityPasswordMaxLength: 16,
			},
		},
		// Flags reported as disabled are left out
		{d{k(support.ARSupportValueChpass): 0, k(support.ARSupportValueSecureDefault): 0}, caps{}},
		// A zero length is unreported, not a limit of zero
		{d{k(support.ARSupportValueHttpUsernameMaxLength): "0", k(support.ARSupportValueHttpPasswordMaxLength): "0"}, caps{}},
		// Legacy firmware advertises the lengths but has no CHPASS
		{d{k(support.ARSupportValueHttpPasswordMaxLength): "32"}, caps{credentials.ARCredentialsCapabilityPasswordMaxLength: 32}},
	})
}
