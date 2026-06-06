package fluid

import (
	"math"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
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
	previous, updated []krakenmarket.BookLevel,
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

/*
trustedImbalanceLocked sums visible depth while excluding toxic resting size.
*/
func (state *FluidSymbol) trustedImbalanceLocked(
	bids, asks []krakenmarket.BookLevel,
	at time.Time,
) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	bidVolume := 0.0
	askVolume := 0.0

	for _, level := range bids {
		if state.isToxicLevelLocked(level.Price, at) {
			continue
		}

		bidVolume += level.Qty
	}

	for _, level := range asks {
		if state.isToxicLevelLocked(level.Price, at) {
			continue
		}

		askVolume += level.Qty
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return 0, false
	}

	return (bidVolume - askVolume) / total, true
}

/*
vorticityLocked combines aggressor buy pressure with the volume-clocked churn ratio
only when near-touch toxicity is absent and a completed flux bar exists.
*/
func (state *FluidSymbol) vorticityLocked(at time.Time) float64 {
	if toxicity.NearTouchToxic(state.symbol, at) {
		return state.buyPressure
	}

	bookFlux, tradeFlux, err := state.flux.completedBar()

	if err != nil {
		return state.buyPressure
	}

	return state.buyPressure * (1 + bookFlux/tradeFlux)
}
