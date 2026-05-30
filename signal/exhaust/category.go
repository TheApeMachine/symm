package exhaust

import "github.com/theapemachine/symm/market/perspectives"

/*
exhaustCategory maps the dominant exit reason onto the exhaustion perspective.
*/
func exhaustCategory(reason string) perspectives.CategoryType {
	switch reason {
	case reasonBookThinning:
		return perspectives.CategoryMechanicalCollapse
	case reasonSpreadWiden:
		return perspectives.CategoryFragileExpansion
	case reasonPressureFade:
		return perspectives.CategoryThermalExhaustion
	case reasonImbalanceFlip:
		return perspectives.CategoryActiveReversal
	default:
		return perspectives.CategoryThermalExhaustion
	}
}
