package translate

// Disk monitor log events for AsusRouter.

// DiskMonitorPatterns mirrors `disk_monitor.PATTERNS`.
var DiskMonitorPatterns = NewARLogPatternSet(
	AREventDiskMonitorUnknown,
	MustARLogPattern(AREventDiskMonitorFinish, `^Finish$`, PatternOptions{Marker: "Finish"}),
	// The scan timer fired
	MustARLogPattern(
		AREventDiskMonitorSigalrm, `Got SIGALRM`, PatternOptions{Marker: "Got SIGALRM"},
	),
)
