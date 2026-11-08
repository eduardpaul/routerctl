package translate

// Daemon lifecycle log events for AsusRouter.

// DaemonPatterns mirrors `daemon.PATTERNS`.
// Shared by every daemon reporting only that it came up or went down
var DaemonPatterns = NewARLogPatternSet(
	AREventDaemonUnknown,
	// Matching the verb stem covers the firmware's own `stoped`
	MustARLogPattern(
		map[string]StrEnum{
			"start": AREventDaemonStarted,
			"stop":  AREventDaemonStopped,
		},
		`daemon is (?P<event_type>start|stop)`,
		PatternOptions{Marker: "daemon is"},
	),
)
