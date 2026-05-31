package cmd

import (
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/trader/economics"
)

/*
TuneFitness is the scalar score symm tune maximizes.
It rewards realized wallet PnL, penalizes profitable entries blocked by gates,
and discounts profitable scores that take longer to realize.
*/
func TuneFitness(
	scoreEUR float64,
	missedForwardEUR float64,
	performance economics.PerformanceSummary,
) float64 {
	if scoreEUR <= 0 {
		return scoreEUR - missedForwardEUR
	}

	return scoreEUR/(1+profitHoldRatio(performance)) - missedForwardEUR
}

func profitHoldRatio(performance economics.PerformanceSummary) float64 {
	if performance.ProfitableTrades == 0 || performance.MeanProfitHoldMS <= 0 {
		return 0
	}

	ttl := config.System.PerspectiveTTL

	if ttl <= 0 {
		return 0
	}

	ratio := performance.MeanProfitHoldMS / float64(ttl.Milliseconds())

	if ratio < 0 {
		return 0
	}

	return ratio
}
