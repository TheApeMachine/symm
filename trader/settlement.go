package trader

import (
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

/*
spotProceedsEUR is quote notional for one spot fill.
*/
func spotProceedsEUR(baseQty, fillPrice float64) float64 {
	if baseQty <= 0 || fillPrice <= 0 {
		return 0
	}

	return baseQty * fillPrice
}

/*
spotTakerFeeEUR matches Kraken taker fee on fill proceeds.
*/
func spotTakerFeeEUR(proceeds, feePct float64) float64 {
	if proceeds <= 0 {
		return 0
	}

	return config.System.TakerFee(proceeds, feePct)
}

/*
spotLongEntryCost is cash debited for one long entry including entry fee.
*/
func spotLongEntryCost(baseQty, fillPrice, feePct float64) (proceeds, fee, totalCost float64) {
	proceeds = spotProceedsEUR(baseQty, fillPrice)
	fee = spotTakerFeeEUR(proceeds, feePct)
	totalCost = proceeds + fee

	return proceeds, fee, totalCost
}

func positionExitProceeds(position *Position, exitFill float64) float64 {
	proceeds := spotProceedsEUR(position.BaseQty, exitFill)

	if proceeds > 0 {
		return proceeds
	}

	if position.FillPrice <= 0 {
		return 0
	}

	return position.NotionalEUR * (exitFill / position.FillPrice)
}

func positionMarkValue(position *Position, last float64) float64 {
	if position.Side == positionShort {
		if position.FillPrice <= 0 {
			return 0
		}

		return position.NotionalEUR * (position.FillPrice - last) / position.FillPrice
	}

	proceeds := spotProceedsEUR(position.BaseQty, last)

	if proceeds > 0 {
		return proceeds
	}

	if position.FillPrice <= 0 {
		return 0
	}

	return position.NotionalEUR * (last / position.FillPrice)
}

/*
StopLimitBelow returns the stop-loss-limit floor used on Kraken OTO entries.
*/
func StopLimitBelow(triggerPrice float64) float64 {
	limitBelow := triggerPrice * 0.999

	if limitBelow <= 0 {
		return triggerPrice
	}

	return limitBelow
}

/*
StopLossLimitFill models a triggered stop-loss-limit sell on spot long.
*/
func StopLossLimitFill(
	last, triggerPrice, limitPrice float64,
	bid, ask float64,
	baseQty float64,
	bidLevels []market.BookLevel,
) float64 {
	if last > triggerPrice || triggerPrice <= 0 || limitPrice <= 0 || baseQty <= 0 {
		return 0
	}

	notional := baseQty * last

	if notional <= 0 {
		return 0
	}

	marketFill := config.System.SlippageFill(
		last, bid, ask, "sell", config.System.SlippageBPS,
		notional, bidLevels, nil,
	)

	if marketFill <= 0 {
		return limitPrice
	}

	if marketFill < limitPrice {
		return limitPrice
	}

	return marketFill
}
