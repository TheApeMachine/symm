package replay

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

type bookFillQuote struct {
	price         float64
	slippageBps   float64
	depthCoverage float64
}

/*
walkBookFill mirrors broker.SlippageFill for replay without importing broker,
which would cycle through market/story.
*/
func walkBookFill(
	measurement perspectives.Measurement,
	side trading.Side,
	quantity float64,
) (bookFillQuote, error) {
	if quantity <= 0 {
		return bookFillQuote{}, fmt.Errorf("replay slippage: quantity must be positive")
	}

	reference := measurement.Last

	if reference <= 0 && measurement.Bid > 0 && measurement.Ask > 0 {
		reference = (measurement.Bid + measurement.Ask) / 2
	}

	if reference <= 0 {
		return bookFillQuote{}, fmt.Errorf("replay slippage: missing reference price for %s", measurement.Symbol)
	}

	book := measurement.MarketBook()
	levels := book.Asks

	if side == trading.Sell {
		levels = book.Bids
	}

	if len(levels) == 0 {
		return halfSpreadBookFill(measurement, side, reference), nil
	}

	filledQty := 0.0
	cost := 0.0

	for _, level := range levels {
		if level.Price <= 0 || level.Qty <= 0 {
			continue
		}

		remaining := quantity - filledQty
		takeQty := level.Qty

		if takeQty > remaining {
			takeQty = remaining
		}

		cost += takeQty * level.Price
		filledQty += takeQty

		if filledQty >= quantity {
			break
		}
	}

	if filledQty <= 0 {
		return halfSpreadBookFill(measurement, side, reference), nil
	}

	avgPrice := cost / filledQty
	bestPrice := levels[0].Price
	slippageBps := slippageBpsFromBest(side, bestPrice, avgPrice)

	if filledQty < quantity {
		remainder := quantity - filledQty
		fallback := halfSpreadBookFill(measurement, side, reference)
		blended := (avgPrice*filledQty + fallback.price*remainder) / quantity
		slippageBps = slippageBpsFromBest(side, bestPrice, blended)

		return bookFillQuote{
			price:         blended,
			slippageBps:   slippageBps,
			depthCoverage: filledQty / quantity,
		}, nil
	}

	return bookFillQuote{
		price:         avgPrice,
		slippageBps:   slippageBps,
		depthCoverage: 1,
	}, nil
}

func halfSpreadBookFill(
	measurement perspectives.Measurement,
	side trading.Side,
	reference float64,
) bookFillQuote {
	mid := reference
	spreadBps := 0.0

	if measurement.Bid > 0 && measurement.Ask > 0 && measurement.Ask >= measurement.Bid {
		mid = (measurement.Bid + measurement.Ask) / 2
		spreadBps = (measurement.Ask - measurement.Bid) / mid * 10_000 / 2
	}

	price := mid

	if side == trading.Buy {
		price = mid * (1 + spreadBps/10_000)
	}

	if side == trading.Sell {
		price = mid * (1 - spreadBps/10_000)
	}

	return bookFillQuote{
		price:         price,
		slippageBps:   spreadBps,
		depthCoverage: 0,
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
