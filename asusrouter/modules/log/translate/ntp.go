package translate

// NTP log events for AsusRouter.

// NtpPatterns mirrors `ntp.PATTERNS`.
var NtpPatterns = NewARLogPatternSet(
	AREventNtpUnknown,
	MustARLogPattern(
		AREventNtpStartUpdate,
		`start NTP update`,
		PatternOptions{Marker: "start NTP update"},
	),
)
