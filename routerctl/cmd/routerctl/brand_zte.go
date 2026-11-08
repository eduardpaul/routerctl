//go:build !nozte

package main

// The ZTE driver (build with -tags nozte to leave it out).

import ztedriver "github.com/eduardpaul/routerctl/routerkit/drivers/zte"

func init() { brands = append(brands, ztedriver.Factory()) }
