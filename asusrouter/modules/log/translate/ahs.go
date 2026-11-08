package translate

// Ahs daemon log events for AsusRouter.

// AhsPatterns mirrors `ahs.PATTERNS`.
var AhsPatterns = NewARLogPatternSet(
	AREventAhsUnknown,
	MustARLogPattern(
		AREventAhsJsonUpdate,
		`Update ahs JSON file`,
		PatternOptions{Marker: "Update ahs JSON file"},
	),
	MustARLogPattern(
		AREventAhsTerminate,
		`Terminate ahs daemon`,
		PatternOptions{Marker: "Terminate ahs daemon"},
	),
)
