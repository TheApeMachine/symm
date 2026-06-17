package depthflow

import (
	"math"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
	marketsection "github.com/theapemachine/symm/market"
	feed "github.com/theapemachine/symm/signal"
)

type bookLevel struct {
	price float64
	qty   float64
}

type BookSnapshot struct {
	Weighted        float64
	Level1          float64
	Flat            float64
	FlatOK          bool
	Mid             float64
	Spread          float64
	TouchDepth      float64
	Observed        time.Time
	Elapsed         float64
	WeightedHistory []float64
	Level1History   []float64
	FlatHistory     []float64
}

func bookSnapshot(
	crossSection *marketsection.CrossSection,
	book *feed.Book,
	symbol string,
) BookSnapshot {
	var (
		weightedHistory []float64
		level1History   []float64
		flatHistory     []float64
		snapshotElement []byte
		weighted        float64
		level1          float64
		flat            float64
		flatOK          bool
		mid             float64
		spread          float64
		touchDepth      float64
		firstAt         time.Time
		latestAt        time.Time
	)

	book.Scan(symbol, func(element []byte) {
		if len(element) == 0 {
			return
		}

		eventAt, eventOK := feed.ElementTime(element, "timestamp")

		if eventOK {
			if firstAt.IsZero() {
				firstAt = eventAt
			}

			latestAt = eventAt
		}

		_, bidOK := feed.PeekElementOK[float64](element, "bids.0.price")
		_, askOK := feed.PeekElementOK[float64](element, "asks.0.price")

		if bidOK && askOK {
			snapshotElement = append([]byte(nil), element...)
		}

		if len(snapshotElement) == 0 {
			return
		}

		bids := bookLevels(element, "bids")

		if len(bids) == 0 {
			bids = bookLevels(snapshotElement, "bids")
		}

		asks := bookLevels(element, "asks")

		if len(asks) == 0 {
			asks = bookLevels(snapshotElement, "asks")
		}

		if len(bids) == 0 || len(asks) == 0 {
			return
		}

		touchMid := (bids[0].price + asks[0].price) / 2
		touchSpread := asks[0].price - bids[0].price

		snapshotBids := bookLevels(snapshotElement, "bids")
		snapshotAsks := bookLevels(snapshotElement, "asks")

		frameWeighted, frameWeightedOK := weightedImbalance(
			snapshotBids, snapshotAsks, touchMid, touchSpread,
		)
		frameLevel1, frameLevel1OK := level1Imbalance(bids, asks)
		frameFlat, frameFlatOK := flatImbalance(snapshotBids, snapshotAsks)

		if frameWeightedOK {
			weightedHistory = append(weightedHistory, math.Abs(frameWeighted))
		}

		if frameLevel1OK {
			level1History = append(level1History, math.Abs(frameLevel1))
		}

		if frameFlatOK {
			flatHistory = append(flatHistory, math.Abs(frameFlat))
		}

		weighted = frameWeighted
		level1 = frameLevel1
		flat = frameFlat
		flatOK = frameFlatOK
		mid = touchMid
		spread = touchSpread
		touchDepth = bids[0].qty + asks[0].qty
	})

	if latestAt.IsZero() || mid <= 0 || spread <= 0 || len(weightedHistory) == 0 || len(level1History) == 0 {
		return BookSnapshot{}
	}

	elapsed := 0.0

	if !firstAt.IsZero() {
		elapsed = latestAt.Sub(firstAt).Seconds()
	}

	quoteVol := mid * touchDepth

	if quoteVol > 0 {
		value := math.Abs(weighted) / mid
		row, rowErr := krakenmarket.NewSymbolRow(symbol, mid, value, quoteVol, 0, latestAt)

		if rowErr == nil {
			_ = crossSection.Observe(row)
		}
	}

	return BookSnapshot{
		Weighted:        weighted,
		Level1:          level1,
		Flat:            flat,
		FlatOK:          flatOK,
		Mid:             mid,
		Spread:          spread,
		TouchDepth:      touchDepth,
		Observed:        latestAt,
		Elapsed:         elapsed,
		WeightedHistory: weightedHistory,
		Level1History:   level1History,
		FlatHistory:     flatHistory,
	}
}

func observeTrades(
	crossSection *marketsection.CrossSection,
	trade *feed.Trade,
	symbol string,
) {
	var (
		buyVolume  float64
		sellVolume float64
		prices     []float64
		latestAt   time.Time
	)

	trade.Scan(symbol, func(element []byte) {
		price, priceOK := feed.PeekElementOK[float64](element, "price")
		qty, qtyOK := feed.PeekElementOK[float64](element, "qty")

		if !priceOK || !qtyOK || price <= 0 || qty <= 0 {
			return
		}

		prices = append(prices, price)

		eventAt, eventOK := feed.ElementTime(element, "timestamp")

		if eventOK {
			latestAt = eventAt
		}

		side, _ := feed.PeekElementOK[string](element, "side")

		if side == "buy" {
			buyVolume += qty
		}

		if side == "sell" {
			sellVolume += qty
		}
	})

	gross := buyVolume + sellVolume

	if len(prices) < 2 || gross <= 0 || latestAt.IsZero() {
		return
	}

	pressure := (buyVolume - sellVolume) / gross

	if pressure == 0 {
		return
	}

	quoteVol := gross * prices[len(prices)-1]

	row, rowErr := krakenmarket.SymbolRowFromPrices(symbol, prices, quoteVol, pressure, latestAt)

	if rowErr != nil {
		return
	}

	_ = crossSection.Observe(row)
}

func bookLevels(element []byte, key string) []bookLevel {
	levels := make([]bookLevel, 0, 8)

	feed.EachBookLevelElement(element, key, func(price float64, qty float64) {
		levels = append(levels, bookLevel{price: price, qty: qty})
	})

	return levels
}

func weightedImbalance(
	bids, asks []bookLevel,
	mid, spread float64,
) (float64, bool) {
	if mid <= 0 || spread <= 0 || len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	weightedBid := 0.0
	weightedAsk := 0.0

	for _, level := range bids {
		weight := math.Exp(-math.Abs(level.price-mid) / spread)
		weightedBid += level.qty * weight
	}

	for _, level := range asks {
		weight := math.Exp(-math.Abs(level.price-mid) / spread)
		weightedAsk += level.qty * weight
	}

	total := weightedBid + weightedAsk

	if total <= 0 {
		return 0, false
	}

	return (weightedBid - weightedAsk) / total, true
}

func level1Imbalance(bids, asks []bookLevel) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	total := bids[0].qty + asks[0].qty

	if total <= 0 {
		return 0, false
	}

	return (bids[0].qty - asks[0].qty) / total, true
}

func flatImbalance(bids, asks []bookLevel) (float64, bool) {
	bidVolume := 0.0
	askVolume := 0.0

	for _, level := range bids {
		bidVolume += level.qty
	}

	for _, level := range asks {
		askVolume += level.qty
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return 0, false
	}

	return (bidVolume - askVolume) / total, true
}
