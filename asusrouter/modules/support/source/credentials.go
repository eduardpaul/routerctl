package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/credentials"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

var credentialsCapabilityFlags = []support.TableEntry[credentials.ARCredentialsCapability]{
	{Key: string(support.ARSupportValueChpass), Value: credentials.ARCredentialsCapabilityChpass},
	{Key: string(support.ARSupportValueSecureDefault), Value: credentials.ARCredentialsCapabilitySecureDefault},
}

var credentialsCapabilityLengths = []support.TableEntry[credentials.ARCredentialsCapability]{
	{Key: string(support.ARSupportValueHttpPasswordMaxLength), Value: credentials.ARCredentialsCapabilityPasswordMaxLength},
	{Key: string(support.ARSupportValueHttpUsernameMaxLength), Value: credentials.ARCredentialsCapabilityUsernameMaxLength},
}

// translateCredentialsCapabilities maps advertised login capabilities;
// the MAX_LENGTHs carry ints.
func translateCredentialsCapabilities(data map[string]any) map[credentials.ARCredentialsCapability]any {
	capabilities := map[credentials.ARCredentialsCapability]any{}
	for _, entry := range credentialsCapabilityFlags {
		if readers.IsTrueInDict(entry.Key, data) {
			capabilities[entry.Value] = true
		}
	}
	for _, entry := range credentialsCapabilityLengths {
		// Unreported reads as 0 - leave it out so the caller uses its default
		if length := converters.RawToInt(data[entry.Key]); length != nil && *length != 0 {
			capabilities[entry.Value] = *length
		}
	}

	return capabilities
}
