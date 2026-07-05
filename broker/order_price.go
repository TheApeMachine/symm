package broker

import (
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
)

func (factory *OrderFactory) limitPrice(
	action *logic.Action,
	quote MarketQuote,
	seed orderSeed,
) (float64, error) {
	if !needsLimitPrice(seed.orderType) {
		return 0, nil
	}

	for _, path := range [][]any{{"limit_price"}, {"price"}, {"params", "limit_price"}, {"params", "price"}} {
		if price := actionFloat(action, path...); price > 0 {
			return price, nil
		}
	}

	price := quote.PassivePrice(seed.side)
	if price <= 0 {
		return 0, errnie.Err(errnie.Validation, "broker: limit order missing price for "+seed.symbol, nil)
	}

	return price, nil
}

func (factory *OrderFactory) triggerPrice(
	action *logic.Action,
	seed orderSeed,
) (float64, error) {
	if !needsTriggerPrice(seed.orderType) {
		return 0, nil
	}

	for _, path := range [][]any{{"trigger_price"}, {"stop"}, {"params", "trigger_price"}, {"params", "stop"}} {
		if price := actionFloat(action, path...); price > 0 {
			return price, nil
		}
	}

	return 0, errnie.Err(errnie.Validation, "broker: protective order missing trigger for "+seed.symbol, nil)
}

func (factory *OrderFactory) trailingOffset(
	action *logic.Action,
	seed orderSeed,
) (float64, error) {
	if !needsTrailingOffset(seed.orderType) {
		return 0, nil
	}

	for _, path := range [][]any{{"trailing_stop"}, {"offset"}, {"params", "trailing_stop"}, {"params", "offset"}} {
		if offset := actionFloat(action, path...); offset > 0 {
			return offset, nil
		}
	}

	return 0, errnie.Err(errnie.Validation, "broker: trailing order missing offset for "+seed.symbol, nil)
}

func needsLimitPrice(orderType string) bool {
	return orderType == "limit" || strings.HasSuffix(orderType, "-limit")
}

func needsTriggerPrice(orderType string) bool {
	switch orderType {
	case "stop-loss", "stop-loss-limit", "take-profit", "take-profit-limit":
		return true
	default:
		return false
	}
}

func needsTrailingOffset(orderType string) bool {
	return orderType == "trailing-stop" || orderType == "trailing-stop-limit"
}
