package broker

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/kraken/trading"
)

/*
FillQuote is the simulated execution price and slippage for one order size.
PriceCovered is the book-walk VWAP of ONLY the covered quantity on a partial
fill; Price blends the optimistic half-spread remainder in for full-size
costing. A matcher that reduces the order to the covered quantity must charge
PriceCovered — charging the blend under-priced every partial fill.
*/
type FillQuote struct {
	Price         float64
	PriceCovered  float64
	SlippageBps   float64
	DepthCoverage float64
}

/*
SlippageFill walks the cached L2 book for qty, otherwise half-spread on last.
*/
func SlippageFill(
	quote Quote,
	side trading.Side,
	qty float64,
) (FillQuote, error) {
	if qty <= 0 {
		return FillQuote{}, fmt.Errorf("slippage fill: qty must be positive")
	}

	reference := quote.Last

	if reference <= 0 && quote.Bid > 0 && quote.Ask > 0 {
		reference = (quote.Bid + quote.Ask) / 2
	}

	if reference <= 0 {
		return FillQuote{}, fmt.Errorf("slippage fill: missing reference price for %s", quote.Symbol)
	}

	levels := quote.Book.Asks

	if side == trading.Sell {
		levels = quote.Book.Bids
	}

	if len(levels) == 0 {
		return halfSpreadFill(quote, side, reference), nil
	}

	filledQty := 0.0
	cost := 0.0

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

	if filledQty <= 0 {
		return halfSpreadFill(quote, side, reference), nil
	}

	avgPrice := cost / filledQty
	coverage := filledQty / qty
	bestPrice := levels[0].Price
	slippageBps := slippageBpsFromBest(side, bestPrice, avgPrice)

	if filledQty < qty {
		remainder := qty - filledQty
		fallback := halfSpreadFill(quote, side, reference)
		blended := (avgPrice*filledQty + fallback.Price*remainder) / qty
		slippageBps = slippageBpsFromBest(side, bestPrice, blended)

		return FillQuote{
			Price:         blended,
			PriceCovered:  avgPrice,
			SlippageBps:   slippageBps,
			DepthCoverage: filledQty / qty,
		}, nil
	}

	return FillQuote{
		Price:         avgPrice,
		PriceCovered:  avgPrice,
		SlippageBps:   slippageBps,
		DepthCoverage: coverage,
	}, nil
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
WouldCrossPostOnly reports whether a post-only limit would take liquidity immediately.
*/
func WouldCrossPostOnly(
	quote Quote,
	side trading.Side,
	limitPrice float64,
) bool {
	if limitPrice <= 0 {
		return true
	}

	if side == trading.Buy && quote.Ask > 0 && limitPrice >= quote.Ask {
		return true
	}

	if side == trading.Sell && quote.Bid > 0 && limitPrice <= quote.Bid {
		return true
	}

	return false
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
