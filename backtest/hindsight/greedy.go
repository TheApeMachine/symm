package hindsight

import "time"

/*
Leg is one maximal round trip the tape supports: buy at a trough, sell at a
later peak, no overlapping position. The sum of all legs' profit is the
absolute maximum value the tape contains when every entry and exit is executed
perfectly and at most one lot is held at a time. No external parameter slices
the series; the legs are carved purely from the local extrema the price
actually makes, so the ceiling is a property of the tape, not of a tuning knob.
*/
type Leg struct {
	Symbol    string    `json:"symbol"`
	BuyAt     time.Time `json:"buyAt"`
	BuyPrice  float64   `json:"buyPrice"`
	SellAt    time.Time `json:"sellAt"`
	SellPrice float64   `json:"sellPrice"`
	ProfitPct float64   `json:"profitPct"`
}

/*
computeProfit returns the relative round-trip return of a leg from gross
prices, without fee assumptions. Round-trip arithmetic gives a self-similar
measure: (sell - buy) / buy.
*/
func (leg *Leg) computeProfit() {
	if leg.BuyPrice > 0 {
		leg.ProfitPct = (leg.SellPrice - leg.BuyPrice) / leg.BuyPrice
	}
}

/*
Legs holds the maximal round trips a series supports plus the totals of the
greedy over the same tape. Greedy accumulates every positive step, so it is
strictly ≥ the sum of leg profits and measures the frictionless ceiling a
trader could only reach by trading at infinite frequency; the legs are the
actionable, single-round-trip decomposition.
*/
type Legs struct {
	Symbol string  `json:"symbol"`
	Legs   []Leg   `json:"legs"`
	Greedy float64 `json:"greedy"`
}

/*
RoundTrips extracts the maximal non-overlapping buy-low/sell-high legs from a
price series using a forward monotone swing walk.

The state machine scans once. It keeps the previous price and the candidate
trough of the current rising excursion. A leg is opened when price turns up
after a trough; it is closed at the running peak the moment price gives back
the entire rise it has made since that trough (a full retracement). Because a
buy-then-sell leg that fully retraces to its trough yields zero, closing there
never destroys realised value; the next leg simply restarts from the same
trough price, which equals opening another round trip at the same low.

To avoid describing every tick as an opportunity, a leg is only emitted when
its gross move exceeds round-trip noise: the buy and sell must be distinct
prices with a strictly positive upward excursion. This keeps the ceiling an
upper bound that is still attainable by holding, not a count of per-tick
reversals.
*/
/*
RoundTrips extracts the maximal non-overlapping hold legs from a price series.
The scan is a single forward walk: it remembers the running trough (the best
entry so far) and the highest price after it (the best exit so far). A leg is
only booked when the price makes a strictly lower trough — a better entry that
no long-hold can ignore — and the retained leg is that running trough to its
running peak. Because a dip below the old trough no longer closes a position
by itself, a tape that keeps making higher peaks stays one long hold, exactly
as an executor who holds through noise would experience it.

The state machine scans once. A leg is emitted only when its peak exceeds its
trough and a new lower trough has been found (or the series ends), so the
result never counts per-tick reversals as opportunities. This turns a
multi-peak rally into one position from best entry to best exit, while a new
low below the running entry still splits into a fresh, more profitable hold.
*/
func RoundTrips(series *Series) Legs {
	if series == nil || len(series.Points) < 2 {
		return Legs{Legs: []Leg{}}
	}

	result := Legs{Symbol: series.Symbol, Legs: []Leg{}}

	troughIndex := 0
	peakIndex := 0
	totalGreedy := 0.0

	for i := 1; i < len(series.Points); i++ {
		previous := series.Points[i-1]
		current := series.Points[i]

		// Greedy ceiling: every upward step is harvested.
		if current.Price > previous.Price {
			totalGreedy += current.Price - previous.Price
		}

		// Extend the exit whenever a new high prints after the running
		// trough; dips that stay above the trough are held through.
		if current.Price > series.Points[peakIndex].Price {
			peakIndex = i
		}

		// A strictly lower print is a better entry. Book the hold from the
		// running trough to its peak, then restart from this new trough.
		if current.Price < series.Points[troughIndex].Price {
			closeHeldLeg(&result, series, troughIndex, peakIndex)
			troughIndex = i
			peakIndex = i
		}
	}

	closeHeldLeg(&result, series, troughIndex, peakIndex)

	result.Greedy = totalGreedy

	return result
}

/*
closeHeldLeg commits a trough-to-peak hold, only when it yields positive
profit.
*/
func closeHeldLeg(result *Legs, series *Series, troughIndex, peakIndex int) {
	trough := series.Points[troughIndex]
	peak := series.Points[peakIndex]

	if peak.Price <= trough.Price {
		return
	}

	leg := Leg{
		Symbol:    series.Symbol,
		BuyAt:     trough.At,
		BuyPrice:  trough.Price,
		SellAt:    peak.At,
		SellPrice: peak.Price,
	}
	leg.computeProfit()
	result.Legs = append(result.Legs, leg)
}
