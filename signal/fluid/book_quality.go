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
	if state.fluxPrevious == nil {
		state.fluxPrevious = make(map[float64]float64, len(previous))
	}

	if state.fluxSeen == nil {
		state.fluxSeen = make(map[float64]struct{}, len(updated))
	}

	clear(state.fluxPrevious)
	clear(state.fluxSeen)

	for _, level := range previous {
		state.fluxPrevious[level.Price.Float64()] = level.Qty
	}

	flux := 0.0

	for _, level := range updated {
		price := level.Price.Float64()
		flux += math.Abs(level.Qty - state.fluxPrevious[price])
		state.fluxSeen[price] = struct{}{}
	}

	for price, qty := range state.fluxPrevious {
		if _, seen := state.fluxSeen[price]; seen {
			continue
		}

		flux += qty
	}

	return flux
}
