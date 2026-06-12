package broker

import (
	"fmt"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func krakenOrderType(
	action *logic.Action,
	marginEnabled bool,
	tradingModel string,
) (trading.OrderType, error) {
	if tradingModel == "paper" && action.Type.IsExit() {
		return trading.Market, nil
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
	case logic.ActionTakeProfit:
		return trading.TakeProfit, nil
	case logic.ActionTakeProfitLimit:
		return trading.TakeProfitLimit, nil
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
		return "", fmt.Errorf("broker: unknown action type %q", action.Type)
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
