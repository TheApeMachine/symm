package hindsight

import "time"

/*
Leg is one maximal round trip the tape supports: buy at a trough, sell at a
later peak, with market economics and friction accounted for.
*/
type Leg struct {
	Symbol         string    `json:"symbol"`
	BuyAt          time.Time `json:"buyAt"`
	BuyPrice       float64   `json:"buyPrice"`
	SellAt         time.Time `json:"sellAt"`
	SellPrice      float64   `json:"sellPrice"`
	ProfitPct      float64   `json:"profitPct"`
	GrossProfitPct float64   `json:"grossProfitPct"`
	FrictionPct    float64   `json:"frictionPct"`
}

/*
computeProfit evaluates gross return, market friction, and net economic return.
*/
func (leg *Leg) computeProfit() {
	if leg.BuyPrice > 0 {
		leg.GrossProfitPct = (leg.SellPrice - leg.BuyPrice) / leg.BuyPrice

		if leg.FrictionPct > 0 {
			leg.ProfitPct = leg.GrossProfitPct - leg.FrictionPct
			return
		}

		leg.ProfitPct = leg.GrossProfitPct
	}
}

func (leg *Leg) computeEconomicProfit(trough Point, peak Point) {
	if leg.BuyPrice <= 0 {
		return
	}

	leg.GrossProfitPct = (leg.SellPrice - leg.BuyPrice) / leg.BuyPrice

	entryFriction := trough.Friction

	if entryFriction == 0 && trough.Ask > trough.Bid && trough.Price > 0 {
		entryFriction = (trough.Ask - trough.Bid) / (2 * trough.Price)
	}

	exitFriction := peak.Friction

	if exitFriction == 0 && peak.Ask > peak.Bid && peak.Price > 0 {
		exitFriction = (peak.Ask - peak.Bid) / (2 * peak.Price)
	}

	leg.FrictionPct = entryFriction + exitFriction
	leg.ProfitPct = leg.GrossProfitPct - leg.FrictionPct
}

/*
Legs holds the maximal round trips a series supports plus the totals of the
greedy over the same tape.
*/
type Legs struct {
	Symbol string  `json:"symbol"`
	Legs   []Leg   `json:"legs"`
	Greedy float64 `json:"greedy"`
}

/*
RoundTrips extracts the maximal non-overlapping hold legs from a price series
that yield strictly positive economic returns after market friction.
*/
func RoundTrips(series *Series) Legs {
	if series == nil || len(series.Points) < 2 {
		return Legs{Legs: []Leg{}}
	}

	result := Legs{Symbol: series.Symbol, Legs: []Leg{}}

	troughIndex := 0
	peakIndex := 0
	totalGreedy := 0.0

	for index := 1; index < len(series.Points); index++ {
		previous := series.Points[index-1]
		current := series.Points[index]

		if current.Price > previous.Price {
			totalGreedy += current.Price - previous.Price
		}

		if current.Price > series.Points[peakIndex].Price {
			peakIndex = index
		}

		if current.Price < series.Points[troughIndex].Price {
			closeHeldLeg(&result, series, troughIndex, peakIndex)
			troughIndex = index
			peakIndex = index
		}
	}

	closeHeldLeg(&result, series, troughIndex, peakIndex)

	result.Greedy = totalGreedy

	return result
}

/*
closeHeldLeg commits a trough-to-peak hold only when it yields strictly positive net return after friction.
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
	leg.computeEconomicProfit(trough, peak)

	if leg.ProfitPct > 0 {
		result.Legs = append(result.Legs, leg)
	}
}
