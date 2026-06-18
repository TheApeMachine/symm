package broker

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/kraken/trading"
)

/*
FillQuote is the simulated execution price and slippage for one order size.
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
	quote Quote,
	side trading.Side,
	qty float64,
) (FillQuote, error) {
	if qty <= 0 {
		return FillQuote{}, fmt.Errorf("slippage fill: qty must be positive")
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

func slippageReference(quote Quote) (float64, error) {
	reference := quote.Last

	if reference <= 0 && quote.Bid > 0 && quote.Ask > 0 {
		reference = (quote.Bid + quote.Ask) / 2
	}

	if reference <= 0 {
		return 0, fmt.Errorf("slippage fill: missing reference price for %s", quote.Symbol)
	}

	return reference, nil
}

func slippageLevels(quote Quote, side trading.Side) []BookLevel {
	if side == trading.Sell {
		return quote.Book.Bids
	}

	return quote.Book.Asks
}

func depthSlippageFill(
	quote Quote,
	side trading.Side,
	qty float64,
	reference float64,
	levels []BookLevel,
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

func walkDepthLevels(levels []BookLevel, qty float64) (filledQty, cost float64) {
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
	quote Quote,
	side trading.Side,
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
func ApplyExtraSlippageBps(price float64, side trading.Side, bps float64) float64 {
	if price <= 0 || bps <= 0 {
		return price
	}

	factor := bps / 10_000

	if side == trading.Buy {
		return price * (1 + factor)
	}

	return price * (1 - factor)
}

func halfSpreadFill(quote Quote, side trading.Side, reference float64) FillQuote {
	mid := reference
	spreadBps := 0.0

	if quote.Bid > 0 && quote.Ask > 0 && quote.Ask >= quote.Bid {
		mid = (quote.Bid + quote.Ask) / 2
		spreadBps = (quote.Ask - quote.Bid) / mid * 10_000 / 2
	}

	price := mid

	if side == trading.Buy {
		price = mid * (1 + spreadBps/10_000)
	}

	if side == trading.Sell {
		price = mid * (1 - spreadBps/10_000)
	}

	return FillQuote{
		Price:         price,
		SlippageBps:   spreadBps,
		DepthCoverage: 0,
	}
}

func slippageBpsFromBest(side trading.Side, bestPrice, avgPrice float64) float64 {
	if bestPrice <= 0 || avgPrice <= 0 {
		return 0
	}

	if side == trading.Buy {
		return math.Max(0, (avgPrice-bestPrice)/bestPrice*10_000)
	}

	return math.Max(0, (bestPrice-avgPrice)/bestPrice*10_000)
}

/*
MidSpreadBps returns the half-spread in basis points for round-trip friction checks.
*/
func MidSpreadBps(quote Quote) float64 {
	if quote.Bid <= 0 || quote.Ask <= 0 || quote.Ask < quote.Bid {
		return 0
	}

	mid := (quote.Bid + quote.Ask) / 2

	if mid <= 0 {
		return 0
	}

	return (quote.Ask - quote.Bid) / mid * 10_000 / 2
}
