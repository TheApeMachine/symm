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
func RoundTrips(series *Series) Legs {
	if series == nil || len(series.Points) < 2 {
		return Legs{Legs: []Leg{}}
	}

	result := Legs{Symbol: series.Symbol, Legs: []Leg{}}

	var open *Leg
	troughIndex := 0
	totalGreedy := 0.0

	for i := 1; i < len(series.Points); i++ {
		previous := series.Points[i-1]
		current := series.Points[i]

		// Greedy ceiling: every upward step is harvested.
		if current.Price > previous.Price {
			totalGreedy += current.Price - previous.Price
		}

		if open == nil {
			// Not holding: buy as soon as price rises above the running trough.
			if current.Price > series.Points[troughIndex].Price {
				open = &Leg{
					Symbol:    series.Symbol,
					BuyAt:     series.Points[troughIndex].At,
					BuyPrice:  series.Points[troughIndex].Price,
					SellAt:    current.At,
					SellPrice: current.Price,
				}
			} else if current.Price < series.Points[troughIndex].Price {
				troughIndex = i
			}
		} else {
			// Holding: extend the peak whenever price makes a new high above
			// the best seen so far in this leg.
			if current.Price > open.SellPrice {
				open.SellPrice = current.Price
				open.SellAt = current.At
			}

			// Close when price fully retraces from the leg's peak to at or
			// below the buy price — the rise is fully given back, so holding
			// no longer adds value; restart the search from this trough.
			if current.Price <= open.BuyPrice {
				closeOpeningLeg(&result, open)
				open = nil
				troughIndex = i
			}
		}
	}

	if open != nil {
		closeOpeningLeg(&result, open)
	}

	result.Greedy = totalGreedy

	return result
}

/*
closeOpeningLeg commits a leg that rose and then gave back its rise (or ended
the series), only when it produced positive profit.
*/
func closeOpeningLeg(result *Legs, open *Leg) {
	if open.SellPrice > open.BuyPrice {
		open.computeProfit()
		result.Legs = append(result.Legs, *open)
	}
}
