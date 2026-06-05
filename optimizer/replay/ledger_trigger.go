package replay

import (
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func positionSide(position replayPosition) trading.Side {
	if position.side == trading.Sell {
		return trading.Sell
	}

	return trading.Buy
}

/*
armTrigger attaches a resting protective exit to an open position.
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

func (ledger *replayLedger) checkTriggers(row perspectives.Measurement) {
	ledger.observePrice(row)

	if row.Symbol == "" || row.Last <= 0 {
		return
	}

	position, open := ledger.positions[row.Symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	side := positionSide(position)

	if side == trading.Sell {
		if position.trough == 0 || row.Last < position.trough {
			position.trough = row.Last
			ledger.positions[row.Symbol] = position
		}
	} else if row.Last > position.peak {
		position.peak = row.Last
		ledger.positions[row.Symbol] = position
	}

	if position.triggerType == perspectives.ActionNone {
		return
	}

	extremum := position.peak

	if side == trading.Sell {
		extremum = position.trough
	}

	level, breached := triggerLevel(position, side, extremum, row.Last)

	if !breached {
		return
	}

	ledger.closeAtTrigger(row.Symbol, side, position.triggerType, level, row, nil)
}

func triggerLevel(
	position replayPosition,
	side trading.Side,
	extremum, price float64,
) (level float64, breached bool) {
	if perspectives.IsTrailingExit(position.triggerType) && position.triggerOffset <= 0 {
		return 0, false
	}

	level = perspectives.ProtectiveLevelForSide(
		side,
		position.triggerType,
		position.entryPrice,
		extremum,
		position.triggerOffset,
	)

	return level, perspectives.ProtectiveBreachedForSide(side, position.triggerType, level, price)
}

func trailingVolatilityMultiple(costs ReplayCosts) float64 {
	if costs.TrailingVolatilityMultiple > 0 {
		return costs.TrailingVolatilityMultiple
	}

	return DefaultTrailingVolatilityMultiple
}

func (ledger *replayLedger) closeAtTrigger(
	symbol string,
	side trading.Side,
	actionType perspectives.ActionType,
	level float64,
	row perspectives.Measurement,
	snapshots []perspectives.Measurement,
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
		fillRow := row
		fill := level

		if side == trading.Sell {
			if actionType != perspectives.ActionTakeProfit && row.Last > level {
				fill = row.Last
			}
		} else if actionType != perspectives.ActionTakeProfit && row.Last < level {
			fill = row.Last
		}

		fillRow.Last = fill
		exitFill = ledger.resolveExitFill(side, fillRow, snapshots, position.quantity)
		feePct = ledger.costs.TakerFeePct
	}

	ledger.settle(symbol, exitFill, feePct)
}
