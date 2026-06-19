package response

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/broker"
)

/*
FillQuote is the simulated execution price and slippage for one paper order size.
*/
type FillQuote struct {
	Price         float64
	SlippageBps   float64
	DepthCoverage float64
}

/*
SlippageFill walks cached L2 depth for qty, otherwise half-spread on last.
*/
func SlippageFill(
	quote broker.Quote,
	side string,
	qty float64,
) (FillQuote, error) {
	if qty <= 0 {
		return FillQuote{}, fmt.Errorf("paper slippage: qty must be positive")
	}

	reference, refErr := slippageReference(quote)

	if refErr != nil {
		return FillQuote{}, refErr
	}

	levels := slippageLevels(quote, side)

	if len(levels) == 0 {
		return halfSpreadFill(quote, side, reference), nil
	}

	return depthSlippageFill(quote, side, qty, reference, levels)
}

func slippageReference(quote broker.Quote) (float64, error) {
	reference := quote.Last

	if reference <= 0 && quote.Bid > 0 && quote.Ask > 0 {
		reference = (quote.Bid + quote.Ask) / 2
	}

	if reference <= 0 {
		return 0, fmt.Errorf("paper slippage: missing reference price for %s", quote.Symbol)
	}

	return reference, nil
}

func slippageLevels(quote broker.Quote, side string) []broker.BookLevel {
	if side == "sell" {
		return quote.Book.Bids
	}

	return quote.Book.Asks
}

func depthSlippageFill(
	quote broker.Quote,
	side string,
	qty float64,
	reference float64,
	levels []broker.BookLevel,
) (FillQuote, error) {
	filledQty, cost := walkDepthLevels(levels, qty)

	if filledQty <= 0 {
		return halfSpreadFill(quote, side, reference), nil
	}

	avgPrice := cost / filledQty
	bestPrice := levels[0].Price
	slippageBps := slippageBpsFromBest(side, bestPrice, avgPrice)

	if filledQty < qty {
		return partialDepthFill(
			quote, side, qty, reference, levels[0].Price, filledQty, avgPrice, slippageBps,
		), nil
	}

	return FillQuote{
		Price:         avgPrice,
		SlippageBps:   slippageBps,
		DepthCoverage: filledQty / qty,
	}, nil
}

func walkDepthLevels(levels []broker.BookLevel, qty float64) (filledQty, cost float64) {
	for _, level := range levels {
		if level.Price <= 0 || level.Qty <= 0 {
			continue
		}

		remaining := qty - filledQty
		takeQty := level.Qty

		if takeQty > remaining {
			takeQty = remaining
		}

		cost += takeQty * level.Price
		filledQty += takeQty

		if filledQty >= qty {
			break
		}
	}

	return filledQty, cost
}

func partialDepthFill(
	quote broker.Quote,
	side string,
	qty float64,
	reference float64,
	bestPrice float64,
	filledQty float64,
	avgPrice float64,
	slippageBps float64,
) FillQuote {
	remainder := qty - filledQty
	fallback := halfSpreadFill(quote, side, reference)
	blended := (avgPrice*filledQty + fallback.Price*remainder) / qty
	slippageBps = slippageBpsFromBest(side, bestPrice, blended)

	return FillQuote{
		Price:         blended,
		SlippageBps:   slippageBps,
		DepthCoverage: filledQty / qty,
	}
}

/*
ApplyExtraSlippageBps worsens a fill price by configured paper slippage bps.
*/
func ApplyExtraSlippageBps(price float64, side string, bps float64) float64 {
	if price <= 0 || bps <= 0 {
		return price
	}

	factor := bps / 10_000

	if side == "buy" {
		return price * (1 + factor)
	}

	return price * (1 - factor)
}

func halfSpreadFill(quote broker.Quote, side string, reference float64) FillQuote {
	mid := reference
	spreadBps := 0.0

	if quote.Bid > 0 && quote.Ask > 0 && quote.Ask >= quote.Bid {
		mid = (quote.Bid + quote.Ask) / 2
		spreadBps = (quote.Ask - quote.Bid) / mid * 10_000 / 2
	}

	price := mid

	if side == "buy" {
		price = mid * (1 + spreadBps/10_000)
	}

	if side == "sell" {
		price = mid * (1 - spreadBps/10_000)
	}

	return FillQuote{
		Price:         price,
		SlippageBps:   spreadBps,
		DepthCoverage: 0,
	}
}

func slippageBpsFromBest(side string, bestPrice, avgPrice float64) float64 {
	if bestPrice <= 0 || avgPrice <= 0 {
		return 0
	}

	if side == "buy" {
		return math.Max(0, (avgPrice-bestPrice)/bestPrice*10_000)
	}

	return math.Max(0, (bestPrice-avgPrice)/bestPrice*10_000)
}

func midSpreadBps(quote broker.Quote) float64 {
	if quote.Bid <= 0 || quote.Ask <= 0 || quote.Ask < quote.Bid {
		return 0
	}

	mid := (quote.Bid + quote.Ask) / 2

	if mid <= 0 {
		return 0
	}

	return (quote.Ask - quote.Bid) / mid * 10_000 / 2
}
