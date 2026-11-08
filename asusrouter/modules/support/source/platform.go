package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/platform"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

var translatePlatform = support.MakeEnumTranslator([]support.TableEntry[platform.ARPlatform]{
	{Key: string(support.ARSupportValuePlatformBroadcom), Value: platform.ARPlatformBroadcom},
	{Key: string(support.ARSupportValuePlatformMediatek), Value: platform.ARPlatformMediatek},
	{Key: string(support.ARSupportValuePlatformQualcomm), Value: platform.ARPlatformQualcomm},
	{Key: string(support.ARSupportValuePlatformLantiq), Value: platform.ARPlatformLantiq},
}, platform.ARPlatformUnknown)
