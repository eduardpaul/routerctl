package translate

// Avahi daemon log events for AsusRouter.

// AvahiDaemonPatterns mirrors `avahi_daemon.PATTERNS`.
var AvahiDaemonPatterns = NewARLogPatternSet(
	AREventAvahiDaemonUnknown,
	// The mDNS alias the device announces itself under
	MustARLogPattern(
		AREventAvahiDaemonAliasEstablished,
		`Alias name \"(?P<alias>[^\"]+)\" successfully established`,
		PatternOptions{Marker: "Alias name"},
	),
	MustARLogPattern(
		AREventAvahiDaemonNssUnsupported,
		`No NSS support for mDNS detected`,
		PatternOptions{Marker: "No NSS support"},
	),
)
