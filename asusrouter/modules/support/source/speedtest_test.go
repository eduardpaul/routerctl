package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateSpeedtest(t *testing.T) {
	check(t, "translateSpeedtest", translateSpeedtest, []boolCase{
		{d{k(support.ARSupportValueSpeedtest): 1}, true},
		{d{k(support.ARSupportValueSpeedtest): "0"}, false},
		{d{}, false},
	})
}

func TestTranslateSpeedtestCapabilities(t *testing.T) {
	check(t, "translateSpeedtestCapabilities", translateSpeedtestCapabilities, []struct {
		data     d
		expected []speedtest.ARSpeedTestCapability
	}{
		{d{k(support.ARSupportValueSpeedtest10g): 1}, []speedtest.ARSpeedTestCapability{speedtest.ARSpeedTestCapabilitySpeed10g}},
		{d{}, []speedtest.ARSpeedTestCapability{}},
	})
}
