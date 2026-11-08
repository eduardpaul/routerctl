package fake_test

import (
	"testing"
	"testing/fstest"

	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/fake"
	"github.com/eduardpaul/routerctl/routerkit/drivertest"
)

func TestConformance(t *testing.T) {
	drivertest.Run(t, drivertest.Harness{
		Factory: fake.Factory(),
		New:     func(*testing.T) routerkit.Driver { return fake.New() },
		Unreachable: func(*testing.T) routerkit.Driver {
			f := fake.New()
			f.SetUnreachable(fake.ErrUnreachable)
			return f
		},
		PartFailure: func(*testing.T) (routerkit.Driver, routerkit.Part, routerkit.Part) {
			f := fake.New()
			f.SetPartError(routerkit.PartSystem, "boom")
			return f, routerkit.PartSystem, routerkit.PartWAN
		},
		Homepages: fstest.MapFS{"fake.html": {Data: []byte("<html><title>Fake Router</title>" + fake.Marker + "</html>")}},
		Expect: drivertest.Expect{
			NonEmpty:       routerkit.AllParts,
			Unset:          []string{"firmware.available"},
			Golden:         "testdata/FAKE-1000/snapshot.golden.json",
			RunSafeActions: true,
		},
	})
}
