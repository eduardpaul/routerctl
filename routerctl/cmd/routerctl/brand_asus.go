//go:build !noasus

package main

// The ASUS driver (build with -tags noasus to leave the ASUS library out).

import "github.com/eduardpaul/routerctl/routerkit/drivers/asus"

func init() { brands = append(brands, asus.Factory()) }
