package hawkes

import (
	"math"

	"github.com/theapemachine/symm/engine"
)

/*
hawkesConfidence scores trade-cluster excitation against book confirmation from the
current fit and top-of-book imbalance.
*/
func hawkesConfidence(
	fit BivariateFit,
	asymmetry float64,
	baselineFence float64,
	bookSide float64,
	sellSide bool,
) float64 {
	if asymmetry <= 0 || fit.SpectralRadius >= criticalBranch {
		return 0
	}

	ratio := 0.0

	if sellSide {
		if fit.MuSell <= 0 || fit.SellIntensity <= 0 {
			return 0
		}

		ratio = fit.SellIntensity / fit.MuSell
	}

	if !sellSide {
		if fit.MuBuy <= 0 || fit.BuyIntensity <= 0 {
			return 0
		}

		ratio = fit.BuyIntensity / fit.MuBuy
	}

	if ratio <= baselineFence {
		return 0
	}

	fence := baselineFence

	if fence <= 0 {
		fence = 1
	}

	cluster := asymmetry * engine.ExcessRatio(ratio/fence)
	side := math.Min(math.Abs(bookSide), 1)

	return engine.AlignConfidence(cluster, side)
}
