// Package support mirrors asusrouter/modules/support/.
//
// Only the registry-free parts live here: the flags, the helpers and the
// `ARSupportSource` type the device identity is keyed by. The
// translators (`ai.py`, `wifi.py`, ...) and the fetch/translate pair of
// `__init__.py` live in `support/source`, because they import modules that
// register sources (see PARITY.md, split packages).
package support

import "github.com/eduardpaul/routerctl/asusrouter/modules/source"

// ARSupportSource is the AsusRouter support data source.
type ARSupportSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARSupportSource) SourceName() string { return "ARSupportSource" }

// ARSupportSourceUniversal is the universal instance - preferred.
var ARSupportSourceUniversal = &ARSupportSource{}
