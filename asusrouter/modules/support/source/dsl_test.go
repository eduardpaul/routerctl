package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateDsl(t *testing.T) {
	check(t, "translateDsl", translateDsl, []boolCase{
		{d{k(support.ARSupportValueDsl): 1}, true},
		{d{k(support.ARSupportValueDsl): "0"}, false},
		{d{}, false},
	})
}
