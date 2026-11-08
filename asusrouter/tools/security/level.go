// Security level (level.py).

package security

import (
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/tools/enum"
)

// ARSecurityLevel mirrors `ARSecurityLevel` (FromIntMixin, IntEnum).
// Security levels for the data processed by AsusRouter.
//
// Levels:
//   - STRICT - no user-related data is exposed outside
//   - DEFAULT - non-sensitive user-related data is exposed
//   - SANITIZED - user-related data is available but is
//     automatically sanitized before being exposed
//   - REASONABLE - reasonably-sensitive user-related data (MAC, IP)
//     is exposed raw, but secrets (passwords) are not
//   - UNSAFE - user-related data is exposed
type ARSecurityLevel int

const (
	ARSecurityLevelUnknown ARSecurityLevel = consts.UnknownMember

	ARSecurityLevelStrict     ARSecurityLevel = 0
	ARSecurityLevelDefault    ARSecurityLevel = 1
	ARSecurityLevelSanitized  ARSecurityLevel = 5
	ARSecurityLevelReasonable ARSecurityLevel = 7
	ARSecurityLevelUnsafe     ARSecurityLevel = 9
)

// ARSecurityLevelMembers lists the members in definition order.
var ARSecurityLevelMembers = []ARSecurityLevel{
	ARSecurityLevelUnknown,
	ARSecurityLevelStrict,
	ARSecurityLevelDefault,
	ARSecurityLevelSanitized,
	ARSecurityLevelReasonable,
	ARSecurityLevelUnsafe,
}

var aRSecurityLevelByName = map[string]ARSecurityLevel{
	"UNKNOWN":    ARSecurityLevelUnknown,
	"STRICT":     ARSecurityLevelStrict,
	"DEFAULT":    ARSecurityLevelDefault,
	"SANITIZED":  ARSecurityLevelSanitized,
	"REASONABLE": ARSecurityLevelReasonable,
	"UNSAFE":     ARSecurityLevelUnsafe,
}

var aRSecurityLevelNames = map[ARSecurityLevel]string{
	ARSecurityLevelUnknown:    "UNKNOWN",
	ARSecurityLevelStrict:     "STRICT",
	ARSecurityLevelDefault:    "DEFAULT",
	ARSecurityLevelSanitized:  "SANITIZED",
	ARSecurityLevelReasonable: "REASONABLE",
	ARSecurityLevelUnsafe:     "UNSAFE",
}

// Name returns the Python member name.
func (e ARSecurityLevel) Name() string { return aRSecurityLevelNames[e] }

// Valid reports whether e is a defined member.
func (e ARSecurityLevel) Valid() bool { _, ok := aRSecurityLevelNames[e]; return ok }

// ARSecurityLevelFromValueOK mirrors `ARSecurityLevel.from_value` without
// the UNKNOWN fallback.
func ARSecurityLevelFromValueOK(value any) (ARSecurityLevel, bool) {
	return enum.FromIntOK(value, ARSecurityLevelMembers, aRSecurityLevelByName)
}

// ARSecurityLevelFromValue mirrors `ARSecurityLevel.from_value`:
// unresolved values map to ARSecurityLevelUnknown.
func ARSecurityLevelFromValue(value any) ARSecurityLevel {
	return enum.FromInt(value, ARSecurityLevelMembers, aRSecurityLevelByName, ARSecurityLevelUnknown)
}
