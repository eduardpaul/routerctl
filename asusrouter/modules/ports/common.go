package ports

import (
	"log/slog"
	"sync"
)

// Raw port labels already reported as unknown, to warn only once each
var (
	reportedUnknownPorts = map[string]struct{}{}
	unknownPortsLock     sync.Mutex
)

// WarnUnknownPort warns once per distinct unknown raw port label.
func WarnUnknownPort(value string) {
	unknownPortsLock.Lock()
	defer unknownPortsLock.Unlock()
	if _, ok := reportedUnknownPorts[value]; ok {
		return
	}
	reportedUnknownPorts[value] = struct{}{}
	slog.Warn("We found an unknown port type. Please, report this value: `" + value + "`")
}
