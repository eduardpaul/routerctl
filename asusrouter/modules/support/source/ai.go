package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/ai"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

var translateAi = support.MakeBoolTranslator(string(support.ARSupportValueAi), false)
var translateAiCapabilities = support.MakeListTranslator([]support.TableEntry[ai.ARAICapability]{
	{Key: string(support.ARSupportValueAiSlm), Value: ai.ARAICapabilitySlm},
	{Key: string(support.ARSupportValueAiUpgradeBeta), Value: ai.ARAICapabilityUpgradeBeta},
	{Key: string(support.ARSupportValueAiResetBeta), Value: ai.ARAICapabilityResetBeta},
})
