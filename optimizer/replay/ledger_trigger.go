package replay

import "github.com/theapemachine/symm/market/perspectives"

/*
armTrigger attaches a resting protective exit to an open position. The most
recent exit gate wins, so a strategy can revise its protection (e.g. tighten a
stop) on later ticks. The act's Offset overrides the dynamic trigger distance for
this position. No-ops when flat.
*/
func (ledger *replayLedger) armTrigger(
	symbol string, act perspectives.Act,
) {
	position, open := ledger.positions[symbol]

	if !open {
		return
	}

	offset, ok := ledger.armOffset(symbol, act)

	if !ok {
		return
	}

	position.triggerType = act.Type
	position.triggerOffset = offset
	ledger.positions[symbol] = position
}

func (ledger *replayLedger) armOffset(
	symbol string,
	act perspectives.Act,
) (float64, bool) {
	if act.Offset > 0 && act.Offset < 1 {
		return act.Offset, true
	}

	if perspectives.IsTrailingExit(act.Type) {
		volatility := ledger.priceVolatility(symbol)

		if volatility <= 0 {
			return 0, false
		}

		return volatility * trailingVolatilityMultiple(ledger.costs), true
	}

	switch act.Type {
	case perspectives.ActionStopLoss, perspectives.ActionStopLossLimit:
		return ledger.costs.StopLossPct, ledger.costs.StopLossPct > 0
	case perspectives.ActionTakeProfit, perspectives.ActionTakeProfitLimit:
		return ledger.costs.TakeProfitPct, ledger.costs.TakeProfitPct > 0
	default:
		return 0, false
	}
}

/*
checkTriggers advances the running peak and closes the position when the price
path breaches an armed protective trigger. Stops and trailing stops cross the
book on breach (eating any gap-through and slippage); the -limit variants rest
as maker orders and fill at their trigger level. Called once per tick.
*/
func (ledger *replayLedger) checkTriggers(row perspectives.Measurement) {
	ledger.observePrice(row)

	if row.Symbol == "" || row.Last <= 0 {
		return
	}

	position, open := ledger.positions[row.Symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	if row.Last > position.peak {
		position.peak = row.Last
		ledger.positions[row.Symbol] = position
	}

	if position.triggerType == perspectives.ActionNone {
		return
	}

	level, breached := triggerLevel(position, row.Last)

	if !breached {
		return
	}

	ledger.closeAtTrigger(row.Symbol, position.triggerType, level, row.Last, row.SpreadBPS)
}

/*
triggerLevel returns the protective trigger price for the armed exit and whether
the current price has breached it. Long positions only.
*/
func triggerLevel(position replayPosition, price float64) (level float64, breached bool) {
	if perspectives.IsTrailingExit(position.triggerType) && position.triggerOffset <= 0 {
		return 0, false
	}

	level = perspectives.ProtectiveLevel(
		position.triggerType,
		position.entryPrice,
		position.peak,
		position.triggerOffset,
	)

	return level, perspectives.ProtectiveBreached(position.triggerType, level, price)
}

func trailingVolatilityMultiple(costs ReplayCosts) float64 {
	if costs.TrailingVolatilityMultiple > 0 {
		return costs.TrailingVolatilityMultiple
	}

	return DefaultTrailingVolatilityMultiple
}

/*
closeAtTrigger realizes a protective exit. Market-style triggers (stop, take,
trailing) cross the book: a stop or trail fills at the worse of the trigger and
the current price (gap-through), paying taker fee and slippage. The -limit
variants rest as maker orders and fill at their trigger level with the maker fee
and no crossing slippage.
*/
func (ledger *replayLedger) closeAtTrigger(
	symbol string,
	actionType perspectives.ActionType,
	level float64,
	price float64,
	spreadBPS float64,
) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	var exitFill, feePct float64

	if perspectives.ExitRestsAsLimit(actionType) {
		exitFill = level
		feePct = ledger.costs.MakerFeePct
	} else {
		fill := level

		// A market stop/trail eats a downside gap-through; a market take-profit
		// does not assume a favourable upside gap.
		if actionType != perspectives.ActionTakeProfit && price < level {
			fill = price
		}

		exitFill = fill * (1 - halfSpreadSlippagePct(ledger.costs, spreadBPS))
		feePct = ledger.costs.TakerFeePct
	}

	ledger.settle(symbol, exitFill, feePct)
}
