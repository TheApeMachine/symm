package fluid

import (
	"math"
	"time"

	"github.com/theapemachine/symm/signal/toxicity"
)

func (state *FluidSymbol) isToxicLevelLocked(price float64, at time.Time) bool {
	return toxicity.IsToxic(state.symbol, price, at)
}

/*
trustedSideChangeFlux measures book churn while excluding resting liquidity the
toxicity tracker has flagged as bluff at that price.
*/
func (state *FluidSymbol) trustedSideChangeFlux(
	previous, updated []BookLevel,
	at time.Time,
) float64 {
	previousByPrice := make(map[float64]float64, len(previous))

	for _, level := range previous {
		if state.isToxicLevelLocked(level.Price, at) {
			continue
		}

		previousByPrice[level.Price] = level.Qty
	}

	flux := 0.0
	seen := make(map[float64]bool, len(updated))

	for _, level := range updated {
		if state.isToxicLevelLocked(level.Price, at) {
			continue
		}

		flux += math.Abs(level.Qty - previousByPrice[level.Price])
		seen[level.Price] = true
	}

	for price, qty := range previousByPrice {
		if seen[price] {
			continue
		}

		flux += qty
	}

	return flux
}
