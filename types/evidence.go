package types

import "github.com/krakenfx/api-go/v2/pkg/decimal"

/*
StopEvidence is one observation of an open lot: the price a sale of it would
actually realise right now, and whether that price is trustworthy enough to
move the stop's geometry.

It carries only what the regulator consumes. Forecast return, causal uplift and
cognition confidence used to be declared here and were never read, because the
decision they belong to — is this thesis still alive — is made by
ManageContinuation, which has all of it in hand already and can act on it
through the ordinary decision path. Duplicating those numbers into the stop
would give two layers a vote on the same question with no way to reconcile
them.

Missing input leaves Present false, and the regulator freezes rather than
inventing prices.
*/
type StopEvidence struct {
	Symbol string `json:"symbol"`
	/*
		ExecutableMark is the estimated sell VWAP for the position's sellable
		quantity, gross of the exit fee but net of the depth impact walking that
		quantity down the book would cost.

		The stop compares against this rather than the top bid because the top
		bid is a price for one unit. A floor defended by a quote that cannot
		absorb the position is not a floor.
	*/
	ExecutableMark *decimal.Decimal `json:"executable_mark"`
	/*
		SellCapacity is the quantity resting at the touch, and Spread and Impact
		are what a crossing costs. Together they re-derive the execution-noise
		band each tick, so the trail can widen with the book instead of holding
		a distance that was measured at entry.
	*/
	SellCapacity *decimal.Decimal `json:"sell_capacity"`
	Spread       *decimal.Decimal `json:"spread"`
	Impact       *decimal.Decimal `json:"impact"`
	/*
		RetreatPressure is cancelled touch qty / prior touch (toxicity). When
		positive, mark moves are quote-only and must not drive stop geometry:
		a quote that retreats upward is not a peak the position could have sold
		into, and ratcheting a floor up to meet it would exit on liquidity that
		was never there.

		It suppresses geometry, not exits. A quote retreating while sell
		capacity collapses is evidence of execution danger, not of safety, and
		the executable mark already carries that half.
	*/
	RetreatPressure float64 `json:"retreat_pressure"`
	/*
		RetreatReady is true when this cut observed a retreat measurement, so
		the regulator can tell "no retreat" from "no reading".
	*/
	RetreatReady bool `json:"retreat_ready"`
	Present      bool `json:"present"`
}

/*
GeometryValid reports whether this observation may set a new peak.

Absent evidence is treated as valid: a mark that arrived without a toxicity
reading is the ordinary case, and freezing the peak whenever the strategy has
not spoken would leave the trail permanently anchored at entry.
*/
func (evidence StopEvidence) GeometryValid() bool {
	return !(evidence.RetreatReady && evidence.RetreatPressure > 0)
}
