package fluid

import (
	"time"
)

/*
MarketFacts carries quote context used to enrich dashboard measurements.
*/
type MarketFacts struct {
	Price      float64
	Volume     float64
	Spread     float64
	Elapsed    float64
	Surprise   float64
	ObservedAt time.Time
}

/*
MarketFacts reads the latest ticker and trade context for one scope.
*/
func (signal *Signal) MarketFacts(scope string) MarketFacts {
	if signal == nil || scope == "" {
		return MarketFacts{}
	}

	tickerSnap := signal.ticker.Snapshot(scope)
	tradeSnap := signal.trade.Snapshot(scope)

	price := tickerSnap.Last

	if price <= 0 {
		price = tradeSnap.Price
	}

	spreadBps := 0.0

	if price > 0 && tickerSnap.Bid > 0 && tickerSnap.Ask > 0 && tickerSnap.Ask >= tickerSnap.Bid {
		spreadBps = (tickerSnap.Ask - tickerSnap.Bid) / price * 10000.0
	}

	volume := tradeSnap.Volume

	if volume <= 0 {
		volume = tickerSnap.Volume
	}

	elapsed := tradeSnap.Elapsed

	if elapsed <= 0 {
		elapsed = tickerSnap.Elapsed
	}

	observedAt := tradeSnap.Observed

	if observedAt.IsZero() {
		observedAt = tickerSnap.Observed
	}

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	surprise := 0.0

	if spreadBps > 0 && price > 0 {
		surprise = spreadBps / 10000.0
	}

	return MarketFacts{
		Price:      price,
		Volume:     volume,
		Spread:     spreadBps,
		Elapsed:    elapsed,
		Surprise:   surprise,
		ObservedAt: observedAt,
	}
}
