package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/platform"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslatePlatform(t *testing.T) {
	check(t, "translatePlatform", translatePlatform, []struct {
		data     d
		expected platform.ARPlatform
	}{
		{d{k(support.ARSupportValuePlatformBroadcom): 1}, platform.ARPlatformBroadcom},
		{d{k(support.ARSupportValuePlatformMediatek): 1}, platform.ARPlatformMediatek},
		{d{k(support.ARSupportValuePlatformQualcomm): 1}, platform.ARPlatformQualcomm},
		{d{k(support.ARSupportValuePlatformLantiq): 1}, platform.ARPlatformLantiq},
		// First match wins
		{d{k(support.ARSupportValuePlatformBroadcom): 1, k(support.ARSupportValuePlatformLantiq): 1}, platform.ARPlatformBroadcom},
		{d{}, platform.ARPlatformUnknown},
	})
}
