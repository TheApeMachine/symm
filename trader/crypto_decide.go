package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
decide applies exchange taker friction to calibrated forecasts, runs Planner
Decide, and publishes the resulting strategy frames. Forecasts leave Analyzer
without FrictionReady so fee provenance stays on the broker Price surface.
*/
func (crypto *Crypto) decide(thesis *types.Thesis) {
	if thesis == nil || crypto.planner == nil {
		return
	}

	crypto.seedOpen(thesis)
	fees := crypto.applyFriction(thesis)
	available, normal, reserved := crypto.constraints()
	crypto.planner.Decide(thesis, fees, available, normal, reserved)
	crypto.publishStrategy(thesis)
}

/*
seedOpen copies broker-open inventory onto the Thesis so Decide's
continuation/exit branch sees real positions instead of an empty slice.
*/
func (crypto *Crypto) seedOpen(thesis *types.Thesis) {
	if crypto.desk == nil {
		return
	}

	for _, holding := range crypto.desk.Holdings() {
		thesis.Positions = append(thesis.Positions, holding)
	}
}

/*
applyFriction writes ExpectedFees as a fractional return cost from the cached
TradeVolume percent and marks FrictionReady only when that fee is present.
*/
func (crypto *Crypto) applyFriction(thesis *types.Thesis) map[string]float64 {
	fees := make(map[string]float64, len(thesis.Forecasts))

	if crypto.price == nil {
		return fees
	}

	for index := range thesis.Forecasts {
		forecast := &thesis.Forecasts[index]
		rate, err := crypto.price.FeeRate(forecast.Symbol)

		if err != nil || rate.Fee == nil {
			continue
		}

		fraction := rate.Fee.Float64() / 100

		if fraction < 0 {
			continue
		}

		forecast.ExpectedFees = fraction
		forecast.FrictionReady = true
		fees[forecast.Symbol] = fraction
	}

	return fees
}

/*
constraints returns quote capital plus normal and reserved slot ceilings for
Decide. Missing balance is treated as zero available rather than inventing cash.
*/
func (crypto *Crypto) constraints() (float64, int, int) {
	normal, reserved := 0, 0

	if crypto.desk != nil {
		normal = crypto.desk.NormalSlots()
		reserved = crypto.desk.ReservedSlots()
	}

	if crypto.balance == nil {
		return 0, normal, reserved
	}

	available, err := crypto.balance.AvailableQuote()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"crypto: quote balance unavailable for decide",
			err,
		))

		return 0, normal, reserved
	}

	return available, normal, reserved
}

/*
publishStrategy forwards decision frames so the terminal stops waiting on an
empty strategy channel after Analyzer already published forecasts.
*/
func (crypto *Crypto) publishStrategy(thesis *types.Thesis) {
	if crypto.uiHub == nil || len(thesis.Decisions) == 0 {
		return
	}

	select {
	case crypto.uiHub.Messages <- datura.Map[any]{
		"decisions": thesis.Decisions,
	}.Marshal():
	default:
	}
}
