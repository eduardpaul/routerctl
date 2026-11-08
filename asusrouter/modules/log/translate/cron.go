package translate

// Cron log events for AsusRouter.

import "github.com/eduardpaul/routerctl/asusrouter/modules/log"

// CronPatterns mirrors `cron.PATTERNS`.
var CronPatterns = NewARLogPatternSet(
	AREventCronUnknown,
	// Minutes the clock jumped away from the base
	MustARLogPattern(
		AREventCronTimeDisparity,
		`time disparity of (?P<minutes>-?\d+) minutes detected`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyMinutes: Int},
			Marker:  "time disparity",
		},
	),
)
