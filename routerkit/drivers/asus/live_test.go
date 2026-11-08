package asus

import (
	"testing"

	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivertest"
)

// TestLiveConformance runs the conformance suite against a real router,
// read-only:
//
//	ROUTERCTL_LIVE=asus ROUTERCTL_LIVE_HOST=192.168.50.1 ROUTERCTL_LIVE_USER=admin ROUTERCTL_LIVE_PASS=... \
//	  go test ./routerkit/drivers/asus -run TestLiveConformance -v
func TestLiveConformance(t *testing.T) {
	cfg, ok := drivertest.LiveConfig(Info.Name)
	if !ok {
		t.Skip("set ROUTERCTL_LIVE=asus and the device credentials to run against a router")
	}
	drivertest.Run(t, drivertest.Harness{
		Factory: Factory(),
		New: func(t *testing.T) routerkit.Driver {
			d, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			return d
		},
		Live: true,
	})
}
