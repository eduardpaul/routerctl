package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/lan"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

var translateLanCapabilities = support.MakeListTranslator([]support.TableEntry[lan.ARLANCapability]{
	{Key: string(support.ARSupportValueLanAggregation), Value: lan.ARLANCapabilityAggregation},
})
