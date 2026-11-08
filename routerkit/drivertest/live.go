package drivertest

import (
	"os"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
)

// LiveConfig returns the device of a live conformance run of driver
// name, enabled with
//
//	ROUTERCTL_LIVE=<name> ROUTERCTL_LIVE_HOST=... ROUTERCTL_LIVE_USER=... ROUTERCTL_LIVE_PASS=...
//
// ok is false when no live run is asked for. Live runs are read-only
// unless LiveWrite.
func LiveConfig(name string) (cfg routerkit.Config, ok bool) {
	get := func(k string) string { return os.Getenv("ROUTERCTL_LIVE_" + k) }
	if os.Getenv("ROUTERCTL_LIVE") != name {
		return routerkit.Config{}, false
	}
	return routerkit.Config{
		Name:     "live",
		Host:     get("HOST"),
		Username: get("USER"),
		Password: get("PASS"),
		Timeout:  30 * time.Second,
	}, true
}

// LiveWrite reports whether a live run may run Safe actions
// (ROUTERCTL_LIVE_WRITE=1). Disruptive and Destructive actions never run.
func LiveWrite() bool { return os.Getenv("ROUTERCTL_LIVE_WRITE") == "1" }
