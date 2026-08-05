package strategy

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
candidate is one symbol's tradeable view of the current tick, assembled from
the thesis evidence and the live book at the moment it is judged.

It is deliberately not a stored type. Nothing carries a candidate across ticks
or publishes it, so it holds only what an evaluation reads and nothing that
would have to be kept calibrated or expired.
*/
type candidate struct {
	Symbol         string
	At             time.Time
	ExpectedReturn *decimal.Decimal
	ReferencePrice *decimal.Decimal
	ExpectedFees   *decimal.Decimal
	ExpectedSpread *decimal.Decimal
	ExpectedImpact *decimal.Decimal
	/*
		TrajectoryTreatment is the first resonance forward-curve step. The causal
		history's treatment column is built from that exact quantity, while
		ExpectedReturn is the accumulated curve converted to quote currency for
		valuation. Keeping them separate prevents MCTS from intervening with a
		multi-step total against a model fitted on one-step predictions.
	*/
	TrajectoryTreatment float64
	Uncertainty         float64
	Confidence          float64
	HorizonSteps        int
	Epoch               uint64
}

/*
projectedReturn converts the resonance rollout's accumulated log return into a
simple return.

The rollout is already the price forecast. Structural signals may corroborate
or contradict that forecast elsewhere in the evaluator, but multiplying the
log return by their unrelated scores turns evidence strength into fictional
price movement and can exponentiate a small forecast into an impossible one.
*/
func projectedReturn(curve, surviving []float64) float64 {
	return math.Expm1(accumulatedReturn(curve, surviving))
}

/*
ExecutableReturn subtracts every modeled execution friction from the expected
market return.
*/
func (row candidate) ExecutableReturn() *decimal.Decimal {
	if row.ExpectedReturn == nil {
		return decimal.NewFromInt64(0).SetScale(8)
	}

	res := row.ExpectedReturn.SetScale(8)

	if row.ExpectedFees != nil {
		res = res.Sub(row.ExpectedFees.SetScale(8))
	}

	if row.ExpectedSpread != nil {
		res = res.Sub(row.ExpectedSpread.SetScale(8))
	}

	if row.ExpectedImpact != nil {
		res = res.Sub(row.ExpectedImpact.SetScale(8))
	}

	return res
}

/*
FractionOf expresses one of the candidate's currency amounts as a fraction of
its reference price, which is the scale every threshold and comparison in the
decision path is stated on.

A missing reference price yields zero rather than an error: the candidate could
not have been priced without one, so this is unreachable for a candidate that
was actually built, and a zero fraction is the reading that claims nothing.
*/
func (row candidate) FractionOf(amount *decimal.Decimal) float64 {
	if amount == nil || row.ReferencePrice == nil || row.ReferencePrice.Sign() <= 0 {
		return 0
	}

	return amount.SetScale(8).Div(row.ReferencePrice.SetScale(8)).Float64()
}

/*
RoundTripFraction is what entering and exiting this candidate costs, as a
fraction of its reference price.

Fees are already priced for both crossings when the candidate is built, and a
taker pays the full spread over a round trip, so both enter whole.
*/
func (row candidate) RoundTripFraction() float64 {
	return row.FractionOf(row.ExpectedFees) +
		row.FractionOf(row.ExpectedSpread) +
		row.FractionOf(row.ExpectedImpact)
}

/*
ExecutableFraction is the executable net return as a fraction of the reference
price, which is the unit every utility in the decision path is stated in.

Utility travels from here to the arbiter, where it is compared against an
incumbent's forward continuation utility and complete exit cost. Those are both
dimensionless fractions, so a utility carried in quote currency would make the
comparison decide by the symbol's price: a dollar of edge on a hundred dollar
symbol would dwarf any percentage an incumbent can show, and every challenger
would win.
*/
func (row candidate) ExecutableFraction() float64 {
	return row.FractionOf(row.ExecutableReturn())
}

/*
UncertaintyFraction prices the forecast's uncertainty in the same units as the
return it is weighed against.
*/
func (row candidate) UncertaintyFraction() float64 {
	if math.IsNaN(row.Uncertainty) || math.IsInf(row.Uncertainty, 0) || row.Uncertainty <= 0 {
		return 0
	}

	return row.Uncertainty
}
