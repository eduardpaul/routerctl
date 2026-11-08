package main

// The drivers built into this binary. Each brand adds itself from its own
// brand_<name>.go file, which a no<name> build tag leaves out; the fake
// driver is always there (tests, demos).

import (
	"fmt"
	"os"
	"sync"

	"github.com/eduardpaul/routerctl/routerctl/internal/app"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/fake"
)

var brands = []routerkit.Factory{fake.Factory()}

var (
	regOnce sync.Once
	reg     *routerkit.Registry
)

// registry returns the driver registry, built on first use (after every
// brand_*.go init has run).
func registry() *routerkit.Registry {
	regOnce.Do(func() {
		r, err := app.Registry(brands...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "routerctl: %v\n", err)
			os.Exit(1)
		}
		reg = r
	})
	return reg
}
