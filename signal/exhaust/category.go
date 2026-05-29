package exhaust

import "github.com/theapemachine/symm/engine"

/*
exhaustCategory maps the dominant exit reason onto the exit-thesis perspective.
*/
func exhaustCategory(reason string) engine.Category {
	switch reason {
	case "book_thinning":
		return engine.CatMechanicalCollapse
	case "spread_widen":
		return engine.CatFragileExpansion
	case engine.ExitReasonPressureFade:
		return engine.CatThermalExhaustion
	case engine.ExitReasonImbalanceFlip:
		return engine.CatActiveReversal
	default:
		return engine.CatThermalExhaustion
	}
}
