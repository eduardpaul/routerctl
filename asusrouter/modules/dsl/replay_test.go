package dsl_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/source"

	"github.com/eduardpaul/routerctl/asusrouter/modules/dsl"
)

func TestReplayParity(t *testing.T) {
	runReplay(t, "ARDSLSource", func() source.DataSource { return &dsl.ARDSLSource{} })
}
