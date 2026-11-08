package ztedriver

import (
	"testing"

	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivertest"
)

// TestLiveConformance runs the conformance suite against a real device,
// read-only:
//
//	ROUTERCTL_LIVE=zte ROUTERCTL_LIVE_HOST=192.168.1.1 ROUTERCTL_LIVE_USER=user ROUTERCTL_LIVE_PASS=... \
//	  go test ./routerkit/drivers/zte -run TestLiveConformance -v
//
// Requests are spaced by 500ms; no bad-credentials check runs (it would count towards
// the device lockout).
func TestLiveConformance(t *testing.T) {
	cfg, ok := drivertest.LiveConfig(Name)
	if !ok {
		t.Skip("set ROUTERCTL_LIVE=zte and the device credentials to run against a device")
	}
	cfg.Options = map[string]string{"min_interval": "500ms", "insecure": "true"}
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
