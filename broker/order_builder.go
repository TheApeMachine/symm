package broker

import (
	"fmt"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

/*
OrderContext carries live quote data required to build venue-correct orders.
*/
type OrderContext struct {
	Mark float64
}

/*
BuildAddOrder maps one playbook action to Kraken add_order params.
Each order family validates its own required fields.
*/
func BuildAddOrder(
	action *logic.Action,
	orderContext OrderContext,
	quantity float64,
	clOrdID string,
	token string,
	constraints *krakenmarket.InstrumentConstraints,
) (*trading.AddParams, error) {
	if action == nil {
		return nil, fmt.Errorf("broker: nil action")
	}

	if quantity <= 0 {
		return nil, fmt.Errorf("broker: order quantity must be positive")
	}

	if clOrdID == "" {
		return nil, fmt.Errorf("broker: client order id required")
	}

	orderType, typeErr := action.Type.KrakenOrderType()

	if typeErr != nil {
		return nil, typeErr
	}

	params := &trading.AddParams{
		OrderType: orderType,
		Side:      action.Side,
		Symbol:    action.Symbol,
		OrderQty:  quantity,
		ClOrdID:   clOrdID,
		Token:     token,
	}

	if constraints != nil && constraints.PriceIncrement > 0 {
		switch action.Type {
		case logic.ActionLimit, logic.ActionIceberg:
			limitPrice, priceErr := krakenmarket.QuantizePrice(action.Price, constraints.PriceIncrement)

			if priceErr != nil {
				return nil, priceErr
			}

			params.LimitPrice = limitPrice

			return params, nil
		}
	}

	switch action.Type {
	case logic.ActionMarket:
		return params, nil
	case logic.ActionSettlePosition:
		return params, nil
	case logic.ActionLimit, logic.ActionIceberg:
		limitPrice := action.Price

		if limitPrice <= 0 {
			return nil, fmt.Errorf("broker: limit order requires price")
		}

		params.LimitPrice = limitPrice

		return params, nil
	case logic.ActionStopLoss, logic.ActionStopLossLimit,
		logic.ActionTakeProfit, logic.ActionTakeProfitLimit,
		logic.ActionTrailingStop, logic.ActionTrailingStopLimit:
		triggerPrice := action.Price

		if triggerPrice <= 0 {
			return nil, fmt.Errorf("broker: triggered order requires trigger price")
		}

		params.Triggers = &trading.Triggers{
			Price:     triggerPrice,
			PriceType: "static",
		}

		if action.Type == logic.ActionStopLossLimit ||
			action.Type == logic.ActionTakeProfitLimit ||
			action.Type == logic.ActionTrailingStopLimit {
			if action.Price <= 0 {
				return nil, fmt.Errorf("broker: triggered limit order requires limit price")
			}

			params.LimitPrice = action.Price
		}

		return params, nil
	default:
		return nil, fmt.Errorf("broker: unsupported action type %q", action.Type.String())
	}
}
