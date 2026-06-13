package broker

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func krakenOrderType(
	action *logic.Action,
	marginEnabled bool,
) (trading.OrderType, error) {
	if action == nil {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: action is required",
			nil,
		))
	}

	switch action.Type {
	case logic.ActionMarket:
		return trading.Market, nil
	case logic.ActionLimit:
		return trading.Limit, nil
	case logic.ActionIceberg:
		return trading.Iceberg, nil
	case logic.ActionStopLoss:
		return trading.StopLoss, nil
	case logic.ActionStopLossLimit:
		return trading.StopLossLimit, nil
	case logic.ActionTakeProfit, logic.ActionTakeProfitLimit:
		// Story soft exits are discretionary closes, not resting exchange orders.
		return trading.Market, nil
	case logic.ActionTrailingStop:
		return trading.TrailingStop, nil
	case logic.ActionTrailingStopLimit:
		return trading.TrailingStopLimit, nil
	case logic.ActionSettlePosition:
		if !marginEnabled {
			return trading.Market, nil
		}

		return trading.SettlePosition, nil
	default:
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: unknown action type",
			errnie.Require(map[string]any{
				"action_type": action.Type,
			}),
		))
	}
}

func isTriggeredOrderType(orderType trading.OrderType) bool {
	switch orderType {
	case trading.StopLoss, trading.StopLossLimit,
		trading.TakeProfit, trading.TakeProfitLimit,
		trading.TrailingStop, trading.TrailingStopLimit:
		return true
	default:
		return false
	}
}
