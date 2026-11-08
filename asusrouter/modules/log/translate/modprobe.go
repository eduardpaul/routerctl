package translate

// Kernel module loader log events for AsusRouter.

// ModprobePatterns mirrors `modprobe.PATTERNS`.
var ModprobePatterns = NewARLogPatternSet(
	AREventModprobeUnknown,
	// A module the build leaves out, asked for at boot
	MustARLogPattern(
		AREventModprobeModuleNotFound,
		`module (?P<module>\S+) not found in modules\.dep`,
		PatternOptions{Marker: "not found in modules.dep"},
	),
)
