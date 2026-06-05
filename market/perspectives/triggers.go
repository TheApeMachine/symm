package perspectives

import "github.com/theapemachine/symm/kraken/trading"

/*
The protective-trigger math, shared by every layer that arms or fills a stop /
take-profit / trailing-stop so they cannot drift: the replay ledger scores against
it, the desk encodes Kraken trigger prices from it, and the paper websocket fills
against it. Long and short positions use the same actions with inverted levels.
*/

// TriggerOffset prefers the per-node offset the playbook armed, falling back to the
// global default when the node did not specify one or specified a nonsensical
// fraction (<=0 or >=1 — e.g. a stop "below" entry by 150% would arm above entry).
func TriggerOffset(perNode, global float64) float64 {
	if perNode > 0 && perNode < 1 {
		return perNode
	}

	return global
}

// ProtectiveLevel is the trigger price for a long protective exit.
func ProtectiveLevel(action ActionType, entry, peak, offset float64) float64 {
	return ProtectiveLevelForSide(trading.Buy, action, entry, peak, offset)
}

/*
ProtectiveLevelForSide returns the trigger price for a protective exit. For longs,
extremum is the running peak; for shorts it is the running trough.
*/
func ProtectiveLevelForSide(
	side trading.Side,
	action ActionType,
	entry, extremum, offset float64,
) float64 {
	if side == trading.Sell {
		switch action {
		case ActionStopLoss, ActionStopLossLimit:
			return entry * (1 + offset)
		case ActionTakeProfit, ActionTakeProfitLimit:
			return entry * (1 - offset)
		case ActionTrailingStop, ActionTrailingStopLimit:
			return extremum * (1 + offset)
		default:
			return 0
		}
	}

	switch action {
	case ActionStopLoss, ActionStopLossLimit:
		return entry * (1 - offset)
	case ActionTakeProfit, ActionTakeProfitLimit:
		return entry * (1 + offset)
	case ActionTrailingStop, ActionTrailingStopLimit:
		return extremum * (1 - offset)
	default:
		return 0
	}
}

// ProtectiveBreached reports whether price has crossed the trigger for a long.
func ProtectiveBreached(action ActionType, level, price float64) bool {
	return ProtectiveBreachedForSide(trading.Buy, action, level, price)
}

/*
ProtectiveBreachedForSide reports whether price has crossed the trigger for the
position side.
*/
func ProtectiveBreachedForSide(
	side trading.Side,
	action ActionType,
	level, price float64,
) bool {
	if side == trading.Sell {
		switch action {
		case ActionTakeProfit, ActionTakeProfitLimit:
			return price <= level
		case ActionStopLoss, ActionStopLossLimit, ActionTrailingStop, ActionTrailingStopLimit:
			return price >= level
		default:
			return false
		}
	}

	switch action {
	case ActionTakeProfit, ActionTakeProfitLimit:
		return price >= level
	case ActionStopLoss, ActionStopLossLimit, ActionTrailingStop, ActionTrailingStopLimit:
		return price <= level
	default:
		return false
	}
}

// ExitRestsAsLimit reports whether a protective exit rests as a maker order at its
// trigger level (the -limit variants) rather than crossing the book on breach.
func ExitRestsAsLimit(action ActionType) bool {
	switch action {
	case ActionStopLossLimit, ActionTakeProfitLimit, ActionTrailingStopLimit:
		return true
	default:
		return false
	}
}

// IsTrailingExit reports whether a protective exit trails the running peak.
func IsTrailingExit(action ActionType) bool {
	switch action {
	case ActionTrailingStop, ActionTrailingStopLimit:
		return true
	default:
		return false
	}
}

// IsProtectiveExit reports whether an action arms a resting protective trigger
// (stop / take / trailing and their -limit variants) rather than filling now.
func IsProtectiveExit(action ActionType) bool {
	switch action {
	case ActionStopLoss, ActionStopLossLimit,
		ActionTakeProfit, ActionTakeProfitLimit,
		ActionTrailingStop, ActionTrailingStopLimit:
		return true
	default:
		return false
	}
}
