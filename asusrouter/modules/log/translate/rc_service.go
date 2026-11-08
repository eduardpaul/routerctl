package translate

// Service dispatch log events for AsusRouter.

import (
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// rcServiceActions splits an action list, dropping what a trailing
// separator leaves (Python returns a tuple).
func rcServiceActions(text string) any {
	actions := []string{}
	for _, action := range strings.Split(text, ";") {
		if stripped := converters.PyStrip(action); stripped != "" {
			actions = append(actions, stripped)
		}
	}
	return actions
}

// rcServiceService maps an action token to a service (arg after a space
// is dropped).
func rcServiceService(action string) common.ARService {
	head := ""
	if parts := converters.PySplit(action); len(parts) > 0 {
		head = parts[0]
	}
	return common.ARServiceFromValue(head)
}

// rcServiceServices maps every action token to its service.
func rcServiceServices(actions any) any {
	list, _ := actions.([]string)
	services := make([]common.ARService, 0, len(list))
	for _, action := range list {
		services = append(services, rcServiceService(action))
	}
	return services
}

// rcServiceText keeps a non-empty token, or nothing.
func rcServiceText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// RcServicePatterns mirrors `rc_service.PATTERNS`.
var RcServicePatterns = NewARLogPatternSet(
	AREventRcServiceUnknown,
	// `<caller> <pid>:notify_rc <action>[;<action>...]`; an action may
	// carry an argument, and may be absent from ARService
	MustARLogPattern(
		AREventRcServiceNotify,
		`(?P<caller>\S+)\s+(?P<pid>\d+):notify_rc\s+(?P<actions>.+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyActions: Plain(rcServiceActions),
				log.AREventKeyPid:     Int,
			},
			Derive: []Derived{
				{Key: log.AREventKeyServices, Source: log.AREventKeyActions, Func: rcServiceServices},
			},
			Marker: "notify_rc",
		},
	),
	// `waitting "<action>"[(last_rc:<action>)] via <caller>` - the
	// firmware's own misspelling; `last_rc` only on newer builds
	MustARLogPattern(
		AREventRcServiceWaiting,
		`waitting\s+"(?P<action>[^"]+)"`+
			`(?:\(last_rc:(?P<last_action>[^)]*)\))?`+
			`\s+via\s+(?P<caller>\S+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyLastAction: Plain(rcServiceText),
			},
			Derive: []Derived{
				{Key: log.AREventKeyLastService, Source: log.AREventKeyLastAction, Func: func(v any) any {
					s, _ := v.(string)
					return rcServiceService(s)
				}},
				{Key: log.AREventKeyService, Source: log.AREventKeyAction, Func: func(v any) any {
					s, _ := v.(string)
					return rcServiceService(s)
				}},
			},
			Marker: "waitting",
		},
	),
)
