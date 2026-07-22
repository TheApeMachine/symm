package tests

import (
	"fmt"
	"math"
	"slices"
)

/*
validateExecutions consumes generated fills against a private copy of the
pre-step Level3 queue, proving every print occurred at then-resting liquidity.
*/
func (validator *Validator) validateExecutions(
	symbol string,
	trades []wireTrade,
) error {
	queues := map[string][]orderState{}

	for side, orders := range validator.orders[symbol] {
		for _, order := range orders {
			queues[side] = append(queues[side], order)
		}

		slices.SortFunc(queues[side], func(left, right orderState) int {
			return compareOrders(side, left, right)
		})
	}

	for _, trade := range trades {
		side := "asks"

		if trade.Side == "sell" {
			side = "bids"
		}

		if len(queues[side]) == 0 {
			return fmt.Errorf("tests: %s trade exceeded resting liquidity for %s", trade.Side, symbol)
		}

		price, err := trade.Price.Float64()

		if err != nil {
			return fmt.Errorf("tests: invalid trade price for %s: %w", symbol, err)
		}

		quantity := queues[side][0].qtyValue
		orderPrice := queues[side][0].priceValue

		if math.Abs(price-orderPrice) > 1e-8 || trade.Qty > quantity {
			return fmt.Errorf("tests: %s trade did not consume the pre-step queue for %s", trade.Side, symbol)
		}

		quantity -= trade.Qty

		if quantity == 0 {
			queues[side] = queues[side][1:]
			continue
		}

		queues[side][0].qtyValue = quantity
	}

	return nil
}

/*
compareOrders applies price-time priority to one reconstructed Level3 side.
*/
func compareOrders(side string, left orderState, right orderState) int {
	if left.priceValue != right.priceValue {
		if side == "bids" && left.priceValue > right.priceValue ||
			side == "asks" && left.priceValue < right.priceValue {
			return -1
		}

		return 1
	}

	if left.priority < right.priority {
		return -1
	}

	if left.priority > right.priority {
		return 1
	}

	return 0
}
