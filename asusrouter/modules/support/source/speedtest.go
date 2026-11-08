package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

var translateSpeedtest = support.MakeBoolTranslator(string(support.ARSupportValueSpeedtest), false)
var translateSpeedtestCapabilities = support.MakeListTranslator([]support.TableEntry[speedtest.ARSpeedTestCapability]{
	{Key: string(support.ARSupportValueSpeedtest10g), Value: speedtest.ARSpeedTestCapabilitySpeed10g},
})
