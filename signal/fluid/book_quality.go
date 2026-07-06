package fluid

import (
	"math"

	"github.com/theapemachine/symm/kraken"
)

/*
trustedSideChangeFlux measures L2 book churn between adjacent observed book
states. Toxicity and L3 bluff evidence belong in separate measurements and
story guards; this method does not infer cancel/fill intent from aggregate L2.
*/
func (state *FluidSymbol) trustedSideChangeFlux(
	previous, updated []kraken.BookLevel,
) float64 {
	previousByPrice := make(map[float64]float64, len(previous))

	for _, level := range previous {
		previousByPrice[level.Price] = level.Qty
	}

	flux := 0.0
	seen := make(map[float64]bool, len(updated))

	for _, level := range updated {
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
