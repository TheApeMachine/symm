package perspectives

import (
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/trading"
)

type UnitType uint8

const (
	UnitNone UnitType = iota
	UnitPercentage
	UnitPips
	UnitPoints
	UnitTicks
	UnitTimeYears
	UnitTimeMonths
	UnitTimeWeeks
	UnitTimeDays
	UnitTimeHours
	UnitTimeMinutes
	UnitTimeSeconds
	UnitTimeMilliseconds
	UnitTimeMicroseconds
	UnitTimeNanoseconds
	UnitConfidence
	UnitSNR
)

type ConditionType uint8

const (
	ConditionNone ConditionType = iota
	ConditionIsTrue
	ConditionIsFalse
	ConditionIsEqual
	ConditionIsNotEqual
	ConditionIsGreaterThan
	ConditionIsLessThan
	ConditionIsGreaterThanOrEqual
	ConditionIsLessThanOrEqual
)

type ActionType uint8

const (
	ActionNone ActionType = iota
	ActionLimit
	ActionMarket
	ActionIceberg
	ActionStopLoss
	ActionStopLossLimit
	ActionTakeProfit
	ActionTakeProfitLimit
	ActionTrailingStop
	ActionTrailingStopLimit
	ActionSettlePosition
)

type Action struct {
	Type     ActionType
	Side     trading.Side
	Symbol   string
	Price    float64
	Quantity float64
}

/*
IsEntryAction reports whether actionType opens a position.
*/
func IsEntryAction(actionType ActionType) bool {
	switch actionType {
	case ActionLimit, ActionMarket, ActionIceberg:
		return true
	default:
		return false
	}
}

/*
IsExitAction reports whether actionType closes a position.
*/
func IsExitAction(actionType ActionType) bool {
	switch actionType {
	case ActionSettlePosition,
		ActionStopLoss,
		ActionStopLossLimit,
		ActionTakeProfit,
		ActionTakeProfitLimit,
		ActionTrailingStop,
		ActionTrailingStopLimit:
		return true
	default:
		return false
	}
}

/*
ActionFromMeasurement builds a live desk order from a tree verdict and row.
Entry quantity is sized from the configured paper wallet notional.
*/
func ActionFromMeasurement(
	actionType ActionType, measurement Measurement,
) Action {
	action := Action{
		Type:   actionType,
		Symbol: measurement.Symbol,
		Price:  measurement.Last,
	}

	if IsEntryAction(actionType) {
		action.Side = trading.Buy
		action.Quantity = entryNotionalQuantity(measurement.Last)
	}

	if IsExitAction(actionType) {
		action.Side = trading.Sell
	}

	return action
}

func entryNotionalQuantity(price float64) float64 {
	if price <= 0 {
		return 0
	}

	quote := strings.ToLower(viper.GetString("market.quote_currency"))

	if quote == "" {
		quote = "eur"
	}

	notional := viper.GetFloat64("trading.paper.wallet_" + quote)

	if notional <= 0 && quote == "eur" {
		notional = viper.GetFloat64("trading.paper.wallet_eur")
	}

	if notional <= 0 {
		errnie.Error(errnie.Require(map[string]any{
			"price":    price,
			"notional": notional,
			"quote":    quote,
		}))

		return 0
	}

	return notional / price
}
