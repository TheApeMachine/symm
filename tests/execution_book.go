package tests

import (
	"math"

	testtypes "github.com/theapemachine/symm/tests/types"
)

/*
executionBook owns finite depth, spread crossing, limit eligibility, and
size-dependent execution price.
*/
type executionBook struct {
	config  testtypes.ExecutionConfig
	symbols map[string]*testtypes.Symbol
}

func newExecutionBook(
	config testtypes.ExecutionConfig,
	symbols []*testtypes.Symbol,
) *executionBook {
	profiles := make(map[string]*testtypes.Symbol, len(symbols))

	for _, symbol := range symbols {
		profiles[symbol.Pair] = symbol
	}

	return &executionBook{config: config, symbols: profiles}
}

/*
Consume walks configured finite levels until the requested fragment or
executable limit is exhausted.
*/
func (book *executionBook) Consume(
	order *executionOrder,
	sample testtypes.Sample,
	maximum float64,
) (float64, float64) {
	profile := book.symbols[order.order.Request.Pair]
	remaining := maximum
	quantity := 0.0
	cost := 0.0
	levels := sample.Asks
	direction := 1.0

	if order.order.Request.Type == "sell" {
		levels = sample.Bids
		direction = -1
	}

	if len(levels) == 0 {
		levels = []testtypes.DepthLevel{{
			Price: sample.Ask, Quantity: sample.AskQty,
		}}

		if order.order.Request.Type == "sell" {
			levels[0] = testtypes.DepthLevel{
				Price: sample.Bid, Quantity: sample.BidQty,
			}
		}
	}

	slippageFraction := book.config.SlippageBasisPoints /
		testtypes.BasisPointDenominator

	for level := range min(book.config.DepthLevels, len(levels)) {
		price := levels[level].Price
		price *= 1 + direction*slippageFraction
		price = roundExecution(price, profile.PricePrecision)

		if !book.withinLimit(order, price) {
			break
		}

		available := levels[level].Quantity
		filled := min(remaining, available)
		quantity += filled
		cost += filled * price
		remaining -= filled

		if remaining <= 0 {
			break
		}
	}

	return roundExecution(quantity, profile.QuantityPrecision), cost
}

func (book *executionBook) withinLimit(
	order *executionOrder,
	price float64,
) bool {
	limit := 0.0

	switch order.order.Request.OrderType {
	case "limit":
		limit = order.order.Price
	case "stop-loss-limit":
		limit = order.order.Price2
	default:
		return true
	}

	if order.order.Request.Type == "buy" {
		return price <= limit
	}

	return price >= limit
}

func roundExecution(value float64, precision int) float64 {
	scale := math.Pow10(precision)

	return math.Round(value*scale) / scale
}
