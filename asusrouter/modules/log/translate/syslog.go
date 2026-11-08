package translate

// Syslog daemon log events for AsusRouter.

// SyslogPatterns mirrors `syslog.PATTERNS`.
var SyslogPatterns = NewARLogPatternSet(
	AREventSyslogUnknown,
	MustARLogPattern(
		AREventSyslogStarted,
		`syslogd started: BusyBox v(?P<version>[\d.]+)`,
		PatternOptions{Marker: "syslogd started"},
	),
)
