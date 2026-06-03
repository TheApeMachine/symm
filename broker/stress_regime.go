package broker

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
DeskRegimeForStress reports whether entries must stay post-only during turbulence.
*/
func (stress SymbolStress) DeskRegimeForStress() DeskRegime {
	if stress.FluidCategory == perspectives.CategoryTurbulent && stress.FluidSNR > 0 {
		return DeskRegimeRestricted
	}

	return DeskRegimeNormal
}

/*
RejectsDiscretionaryEntry reports whether toxicity blocks new discretionary entries.
*/
func (stress SymbolStress) RejectsDiscretionaryEntry() bool {
	if stress.ToxicityCategory != perspectives.CategoryToxicBluff {
		return false
	}

	return stress.ToxicitySNR >= 1
}

/*
EntrySlippageCapBps tightens the configured slippage ceiling under hostile regimes.
*/
func (stress SymbolStress) EntrySlippageCapBps(baseBps float64) float64 {
	if baseBps <= 0 {
		return baseBps
	}

	hostile := stress.hostileStress()

	if hostile <= 0 {
		return baseBps
	}

	return baseBps / (1 + hostile)
}

func (stress SymbolStress) hostileStress() float64 {
	var peak float64

	if stress.ToxicityCategory == perspectives.CategoryToxicBluff ||
		stress.ToxicityCategory == perspectives.CategoryLiquidityVacuum {
		peak = math.Max(peak, stress.ToxicitySNR)
	}

	if stress.SentimentCategory == perspectives.CategorySystemicSlump {
		peak = math.Max(peak, stress.SentimentSNR)
	}

	if stress.FluidCategory == perspectives.CategoryTurbulent {
		peak = math.Max(peak, stress.FluidSNR)
	}

	return peak
}
