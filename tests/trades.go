package tests

import (
	"fmt"
	"math"
	"slices"
	"strconv"
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

		price, _ := trade.Price.Float64()
		quantity, _ := strconv.ParseFloat(queues[side][0].qty, 64)
		orderPrice, _ := strconv.ParseFloat(queues[side][0].price, 64)

		if math.Abs(price-orderPrice) > 1e-8 || trade.Qty > quantity {
			return fmt.Errorf("tests: %s trade did not consume the pre-step queue for %s", trade.Side, symbol)
		}

		quantity -= trade.Qty

		if quantity == 0 {
			queues[side] = queues[side][1:]
			continue
		}

		queues[side][0].qty = strconv.FormatFloat(quantity, 'f', 8, 64)
	}

	return nil
}

/*
compareOrders applies price-time priority to one reconstructed Level3 side.
*/
func compareOrders(side string, left orderState, right orderState) int {
	leftPrice, _ := strconv.ParseFloat(left.price, 64)
	rightPrice, _ := strconv.ParseFloat(right.price, 64)

	if leftPrice != rightPrice {
		if side == "bids" && leftPrice > rightPrice ||
			side == "asks" && leftPrice < rightPrice {
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
