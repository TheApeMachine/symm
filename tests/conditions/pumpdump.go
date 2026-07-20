package conditions

import (
	"math"
	"time"

	"github.com/theapemachine/symm/tests"
	instrumentfixture "github.com/theapemachine/symm/tests/fixtures/instrument"
)

/*
PumpDump builds a coherent trade, book, and ticker cycle with calibration,
compression, ignition, continuation, rejection, recoil, and re-ignition. The
path expresses market facts; it does not encode the signal's equations.
*/
func PumpDump() *tests.Market {
	prices := make([]float64, 0, 15)
	quantities := make([]float64, 0, 15)
	spreads := make([]float64, 0, 15)
	depths := make([]float64, 0, 15)
	price := 100.0

	for range 9 {
		prices = append(prices, price)
		quantities = append(quantities, 10)
		spreads = append(spreads, 0.2)
		depths = append(depths, 1_000)
		price *= 1.001
	}

	prices = append(prices, price)
	quantities = append(quantities, 20)
	spreads = append(spreads, 0.04)
	depths = append(depths, 1_500)

	price *= 1.10
	prices = append(prices, price)
	quantities = append(quantities, 200)
	spreads = append(spreads, 0.2)
	depths = append(depths, 1_500)

	price *= 1.05
	prices = append(prices, price)
	quantities = append(quantities, 100)
	spreads = append(spreads, 0.3)
	depths = append(depths, 1_000)

	price *= 0.80
	prices = append(prices, price)
	quantities = append(quantities, 5)
	spreads = append(spreads, 0.8)
	depths = append(depths, 500)

	price *= 1.001
	prices = append(prices, price)
	quantities = append(quantities, 20)
	spreads = append(spreads, 0.04)
	depths = append(depths, 1_500)

	price *= 1.15
	prices = append(prices, price)
	quantities = append(quantities, 250)
	spreads = append(spreads, 0.2)
	depths = append(depths, 1_500)

	return MarketPath(prices, quantities, spreads, depths)
}

/*
MarketPath produces causally ordered Kraken trade, book, and ticker messages
from aligned price, executed-quantity, touch-spread, and touch-depth paths.
Ticker volume and VWAP are derived from the trades rather than independently
specified, preventing contradictory synthetic market state.
*/
func MarketPath(
	prices []float64,
	quantities []float64,
	spreads []float64,
	depths []float64,
) *tests.Market {
	return marketPath(
		subjectSymbol, 0.0001, prices, quantities, nil, spreads, depths,
	)
}

/*
MarketPathWithSides produces the same coherent trade, book, and ticker path
while allowing the aggressor side sequence to be controlled independently of
price direction. This represents mixed order flow inside a directional market.
*/
func MarketPathWithSides(
	prices []float64,
	quantities []float64,
	sides []string,
	spreads []float64,
	depths []float64,
) *tests.Market {
	if len(sides) != len(prices) {
		panic("conditions: market aggressor sides must align with prices")
	}

	return marketPath(
		subjectSymbol, 0.0001, prices, quantities, sides, spreads, depths,
	)
}

/*
MarketPathWithSidesFor produces a coherent path for an explicit instrument.
The price increment comes from that instrument's venue contract.
*/
func MarketPathWithSidesFor(
	symbol string,
	priceIncrement float64,
	prices []float64,
	quantities []float64,
	sides []string,
	spreads []float64,
	depths []float64,
) *tests.Market {
	if symbol == "" || priceIncrement <= 0 || len(sides) != len(prices) {
		panic("conditions: instrument and aligned market sides are required")
	}

	return marketPath(
		symbol, priceIncrement, prices, quantities, sides, spreads, depths,
	)
}

/*
marketPath assembles one validated multi-stream market from aligned facts.
*/
func marketPath(
	symbol string,
	priceIncrement float64,
	prices []float64,
	quantities []float64,
	sides []string,
	spreads []float64,
	depths []float64,
) *tests.Market {
	pathLength := len(prices)

	if pathLength == 0 || pathLength != len(quantities) ||
		pathLength != len(spreads) || pathLength != len(depths) {
		panic("conditions: market paths must be non-empty and equally sized")
	}

	payloads := make([][]byte, 0, pathLength*3)
	startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	openingPrice := prices[0]
	low := openingPrice
	high := openingPrice
	volume := 0.0
	notional := 0.0
	previousPrice := openingPrice
	previousBid := 0.0
	previousAsk := 0.0

	for index, requestedPrice := range prices {
		price := marketPrice(requestedPrice, priceIncrement)
		quantity := quantities[index]
		spread := spreads[index]
		depth := depths[index]

		if price <= 0 || quantity < 0 || spread <= 0 ||
			spread >= price*2 || depth <= 0 {
			panic("conditions: market price, quantity, spread, or depth is invalid")
		}

		if index == 0 && quantity == 0 {
			panic("conditions: first market observation requires executed quantity")
		}

		at := startedAt.Add(time.Duration(index) * time.Second)
		bid := marketPrice(price-spread/2, priceIncrement)
		ask := marketPrice(price+spread/2, priceIncrement)
		side := "buy"

		if len(sides) > 0 {
			side = sides[index]
		}

		if len(sides) == 0 && price < previousPrice {
			side = "sell"
		}

		if side != "buy" && side != "sell" {
			panic("conditions: market aggressor side must be buy or sell")
		}

		if quantity > 0 {
			volume += quantity
			notional += price * quantity
			payloads = append(payloads, tradePayload(
				symbol, index, at, price, quantity, side,
			))
		}

		payloads = append(payloads, bookPayload(
			symbol, index, at, bid, ask, previousBid, previousAsk, depth,
		))
		low = min(low, price)
		high = max(high, price)
		payloads = append(payloads, tickerPayload(
			symbol, at, price, bid, ask, depth, volume, notional, openingPrice, low, high,
		))
		previousPrice = price
		previousBid = bid
		previousAsk = ask
	}

	return tests.NewMarket().
		Prefix(instrumentfixture.NewFixture(instrumentfixture.SNAPSHOT, 1)).
		Feed(tests.NewStaticSequence(payloads...))
}

/*
bookPayload emits a snapshot first and then incremental touch replacements,
including explicit deletion of the previous best prices.
*/
func bookPayload(
	symbol string,
	index int,
	at time.Time,
	bid float64,
	ask float64,
	previousBid float64,
	previousAsk float64,
	depth float64,
) []byte {
	typ := "update"
	bids := []map[string]any{{
		"price": bid,
		"qty":   depth,
	}}
	asks := []map[string]any{{
		"price": ask,
		"qty":   depth,
	}}

	if index == 0 {
		typ = "snapshot"
	}

	if previousBid > 0 && previousBid != bid {
		bids = append(bids, map[string]any{
			"price": previousBid,
			"qty":   0.0,
		})
	}

	if previousAsk > 0 && previousAsk != ask {
		asks = append(asks, map[string]any{
			"price": previousAsk,
			"qty":   0.0,
		})
	}

	return tests.MarshalFrame(map[string]any{
		"channel": "book",
		"type":    typ,
		"data": []map[string]any{{
			"symbol": symbol,
			"bids":   bids,
			"asks":   asks,
			// ponytail: this sequence is an intentional fixture simplification;
			// replace it with Kraken's real CRC checksum when book validation consumes it.
			"checksum":  index + 1,
			"timestamp": at,
		}},
	})
}

/*
tickerPayload emits one internally coherent cumulative ticker observation.
*/
func tickerPayload(
	symbol string,
	at time.Time,
	price float64,
	bid float64,
	ask float64,
	depth float64,
	volume float64,
	notional float64,
	openingPrice float64,
	low float64,
	high float64,
) []byte {
	change := price - openingPrice

	return tests.MarshalFrame(map[string]any{
		"channel": "ticker",
		"type":    "update",
		"data": []map[string]any{{
			"symbol":     symbol,
			"bid":        bid,
			"bid_qty":    depth,
			"ask":        ask,
			"ask_qty":    depth,
			"last":       price,
			"volume":     volume,
			"vwap":       notional / volume,
			"low":        low,
			"high":       high,
			"change":     change,
			"change_pct": change / openingPrice * 100,
			"timestamp":  at,
		}},
	})
}

/*
marketPrice aligns synthetic quotes to the supplied instrument price increment.
*/
func marketPrice(value float64, priceIncrement float64) float64 {
	return math.Round(value/priceIncrement) * priceIncrement
}
