package broker

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives/types"
)

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

/*
EntryExposureScale returns the fraction of normal entry size allowed by stress.
*/
func (stress SymbolStress) EntryExposureScale() float64 {
	hostile := stress.hostileStress()

	if hostile <= 0 {
		return 1
	}

	return 1 / (1 + hostile)
}

/*
EntryQuantity scales a requested entry quantity by live hostile-flow stress.
*/
func (stress SymbolStress) EntryQuantity(quantity float64) float64 {
	if quantity <= 0 {
		return quantity
	}

	return quantity * stress.EntryExposureScale()
}

func (stress SymbolStress) hostileStress() float64 {
	var peak float64

	if stress.ToxicityCategory == types.CategoryToxicBluff ||
		stress.ToxicityCategory == types.CategoryLiquidityVacuum {
		peak = math.Max(peak, stress.ToxicitySNR)
	}

	if stress.SentimentCategory == types.CategorySystemicSlump {
		peak = math.Max(peak, stress.SentimentSNR)
	}

	if stress.FluidCategory == types.CategoryTurbulent {
		peak = math.Max(peak, stress.FluidSNR)
	}

	if stress.HawkesCategory == types.CategoryFrenzy ||
		stress.HawkesCategory == types.CategorySaturation {
		peak = math.Max(peak, stress.HawkesSNR)
	}

	return peak
}
