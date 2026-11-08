package aura

// The enums of enums.py are generated in enums_gen.go; this file holds
// the hand-written module constants of the same Python file.

// AuraColorSchemes mirrors `AURA_COLOR_SCHEMES`: schemes whose colors are
// user-defined and sent on change.
var AuraColorSchemes = map[ARAuraScheme]bool{
	ARAuraSchemeGradient:  true,
	ARAuraSchemeStatic:    true,
	ARAuraSchemeBreathing: true,
	ARAuraSchemeMarquee:   true,
}
