package types

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
riskScale is the working precision for stop geometry. Every distance here is a
per-unit price, and the operands arrive at whatever precision the venue quoted
them at, so each one is widened before arithmetic: decimal math keeps the
receiver's scale, and a price the venue reported without decimals would round
an entire noise band to zero.
*/
const riskScale = 12

/*
RiskPlan is the geometry one lot is entered under: how far price is allowed to
travel against the entry before the loss is taken, how much of a peak may be
given back, and how much profit an exit has to clear before it is worth
protecting.

It is produced once, before the order is placed, because stop distance and
position size are the same decision. A stop wide enough to survive ordinary
noise only makes sense if the quantity behind it was reduced to keep the loss
at that distance affordable, and a plan that carried only the distance would
leave the sizing free to undo it.

Absent (Present false) means the geometry could not be derived. Consumers hold
whatever they already had rather than substituting a number, because a stop
placed on an invented distance is worse than one that does not move.
*/
type RiskPlan struct {
	Present bool `json:"present"`
	/*
		NoiseBand is one execution-noise band, per unit: what a taker crossing
		costs right now in spread plus modeled depth impact. It is the unit
		every other distance here is stated in, because a move smaller than one
		band is not evidence of anything — it is the cost of looking.
	*/
	NoiseBand *decimal.Decimal `json:"noise_band"`
	/*
		RiskDistance is entry price minus hard floor. It is the widest of the
		volatility band, the execution-noise band, and the venue's own minimum
		tick granularity, so the boundary sits outside every source of movement
		that is not a real adverse excursion.
	*/
	RiskDistance *decimal.Decimal `json:"risk_distance"`
	/*
		TrailDistance is how far below a recorded peak the trailing floor sits.
		It is narrower than RiskDistance: giving back a whole risk distance from
		a peak would turn a winner back into the maximum loss.
	*/
	TrailDistance *decimal.Decimal `json:"trail_distance"`
	/*
		ArmBuffer and LockBuffer are the hysteresis that stops profit protection
		from arming and firing on the same tick. Protection arms ArmBuffer above
		the profit line but locks in only LockBuffer above it, so a mark that
		barely clears break-even does not immediately meet its own floor.

		The invariant is ArmBuffer > LockBuffer > 0.
	*/
	ArmBuffer  *decimal.Decimal `json:"arm_buffer"`
	LockBuffer *decimal.Decimal `json:"lock_buffer"`
	/*
		MinEdge is the per-unit profit an exit must clear above pure break-even
		before the position counts as worth protecting. It also absorbs the
		slippage the depth snapshot cannot see: the book moving between the
		moment the mark was read and the moment the sell actually crosses.
	*/
	MinEdge *decimal.Decimal `json:"min_edge"`
	/*
		MaxLoss is the quote-currency amount this lot is allowed to lose at the
		hard floor. It is what capped the quantity.
	*/
	MaxLoss *decimal.Decimal `json:"max_loss"`
	/*
		ExitFeeRate is the taker rate the exit crossing will pay, as a fraction.
		The profit line divides by (1 − rate) rather than adding the fee on,
		because the fee is charged on the proceeds the sale realises, not on the
		price it was aiming at.
	*/
	ExitFeeRate *decimal.Decimal `json:"exit_fee_rate"`
	/*
		TickSize is the venue's price granularity, used to round every derived
		line onto a price the exchange can actually quote.
	*/
	TickSize *decimal.Decimal `json:"tick_size"`
	/*
		ConfirmMarks is how many consecutive executable marks must sit at or
		below the protected floor before it fires. A single wick through the
		floor is not an exit; a breach that holds is.

		The hard floor ignores this entirely.
	*/
	ConfirmMarks int `json:"confirm_marks"`
	/*
		Multiples are kept so the bands can be re-derived from a later reading
		of the book without carrying the configuration back down to the
		regulator.
	*/
	Multiples RiskMultiples `json:"multiples"`
}

/*
Refresh re-derives the distances that describe the current market from a live
reading, and leaves alone the ones that describe this lot's contract.

The trail may breathe: a giveback distance measured at entry stops describing
the book a few minutes later, and a stop that cannot widen with volatility gets
shaken out by it. The risk distance may not, because the position size was
solved against it — moving it would change how much this lot can lose without
anything having decided to. Neither may the profit buffers, because what counts
as enough profit for this lot was settled when it was entered, and letting a
spread blip raise the bar would exit winners for a reason unrelated to them.

Only the execution-noise band is re-read. The volatility term belongs to the
entry-time risk distance, which is frozen anyway, and the book is the thing
that actually reports a fresh number every tick.

An absent or degenerate reading returns the plan unchanged.
*/
func (plan RiskPlan) Refresh(spread, impact *decimal.Decimal) RiskPlan {
	if !plan.Present {
		return plan
	}

	multiples := plan.Multiples

	if multiples.Trail <= 0 {
		multiples = DefaultRiskMultiples()
	}

	noiseBand := sum(scaled(spread), scaled(impact))
	minimumBand := plan.TickSize.Mul(
		decimal.NewFromInt64(max(1, multiples.MinTicks)),
	)

	if noiseBand == nil || noiseBand.Cmp(minimumBand) < 0 {
		return plan
	}

	plan.NoiseBand = noiseBand
	plan.TrailDistance = largest(multiply(noiseBand, multiples.Trail), minimumBand)

	return plan
}

/*
RiskInputs is what a risk plan is derived from at the moment of sizing. Every
field is read from the candidate being judged and the venue it would trade on,
so nothing here has to be kept calibrated across ticks.
*/
type RiskInputs struct {
	ReferencePrice *decimal.Decimal
	Spread         *decimal.Decimal
	Impact         *decimal.Decimal
	/*
		Uncertainty is the forecast's own dispersion, as a fraction of price.
		It is the closest thing available to an adverse-excursion estimate until
		first-passage outcomes have been collected and calibrated.
	*/
	Uncertainty float64
	TickSize    *decimal.Decimal
	ExitFeeRate *decimal.Decimal
	MaxLoss     *decimal.Decimal
	Multiples   RiskMultiples
}

/*
RiskMultiples are how many execution-noise bands each derived distance is
placed at. They are the only free parameters in the geometry and they exist
because the calibrated replacement does not: the conditional adverse-excursion
quantile these should eventually be read from needs first-passage outcomes that
have not been collected yet.

Everything else here is measured. These are assumed, and they are named and
configured in one place so it stays obvious which is which.
*/
type RiskMultiples struct {
	Risk         float64
	Trail        float64
	Arm          float64
	Lock         float64
	MinEdge      float64
	MinTicks     int64
	ConfirmMarks int
}

/*
DefaultRiskMultiples is the geometry used when configuration does not state
one. The hard floor sits three execution-noise bands out so ordinary crossing
cost cannot reach it, the trail gives back two, and protection arms one band
further out than it locks so arming and firing cannot collide.
*/
func DefaultRiskMultiples() RiskMultiples {
	return RiskMultiples{
		Risk:         3,
		Trail:        2,
		Arm:          2,
		Lock:         1,
		MinEdge:      1,
		MinTicks:     4,
		ConfirmMarks: 3,
	}
}

/*
NewRiskPlan derives one lot's stop geometry from the live book and the
forecast's own dispersion.

Nothing is returned as present unless a reference price and a positive noise
band were both available, because every distance below is a multiple of the
band and a zero band collapses the whole geometry onto the entry price.
*/
func NewRiskPlan(inputs RiskInputs) RiskPlan {
	multiples := inputs.Multiples

	if multiples.Risk <= 0 {
		multiples = DefaultRiskMultiples()
	}

	reference := scaled(inputs.ReferencePrice)

	if reference == nil || reference.Sign() <= 0 {
		return RiskPlan{}
	}

	tick := scaled(inputs.TickSize)

	if tick == nil || tick.Sign() <= 0 {
		tick = reference.Mul(decimal.NewFromFloat64(1e-8))
	}

	noiseBand := sum(scaled(inputs.Spread), scaled(inputs.Impact))

	// A book that quoted no spread at all still costs something to cross, and
	// the venue's own granularity is the smallest that cost can be.
	minimumBand := tick.Mul(decimal.NewFromInt64(max(1, multiples.MinTicks)))

	if noiseBand == nil || noiseBand.Cmp(minimumBand) < 0 {
		noiseBand = minimumBand
	}

	volatilityBand := decimal.NewFromInt64(0).SetScale(riskScale)

	if !math.IsNaN(inputs.Uncertainty) && !math.IsInf(inputs.Uncertainty, 0) &&
		inputs.Uncertainty > 0 {
		volatilityBand = reference.Mul(
			decimal.NewFromFloat64(inputs.Uncertainty).SetScale(riskScale),
		)
	}

	riskDistance := largest(
		volatilityBand,
		multiply(noiseBand, multiples.Risk),
		minimumBand,
	)

	lockBuffer := largest(multiply(noiseBand, multiples.Lock), tick)
	armBuffer := largest(
		multiply(noiseBand, multiples.Arm),
		lockBuffer.Add(tick),
	)

	confirmMarks := multiples.ConfirmMarks

	if confirmMarks < 1 {
		confirmMarks = 1
	}

	return RiskPlan{
		Present:       true,
		NoiseBand:     noiseBand,
		RiskDistance:  riskDistance,
		TrailDistance: largest(multiply(noiseBand, multiples.Trail), minimumBand),
		ArmBuffer:     armBuffer,
		LockBuffer:    lockBuffer,
		MinEdge:       largest(multiply(noiseBand, multiples.MinEdge), tick),
		MaxLoss:       scaled(inputs.MaxLoss),
		ExitFeeRate:   scaled(inputs.ExitFeeRate),
		TickSize:      tick,
		ConfirmMarks:  confirmMarks,
		Multiples:     multiples,
	}
}

/*
MaxQuantity is the largest position this plan can carry without the hard floor
costing more than MaxLoss.

This is the half of the coupling that makes a wide stop affordable. Without it,
widening the boundary far enough to survive ordinary noise just converts every
stopped-out trade into a proportionally larger loss.

An absent plan or an unstated loss budget returns nil, which leaves sizing to
whatever other constraint the caller applies.
*/
func (plan RiskPlan) MaxQuantity() *decimal.Decimal {
	if !plan.Present || plan.MaxLoss == nil || plan.MaxLoss.Sign() <= 0 {
		return nil
	}

	if plan.RiskDistance == nil || plan.RiskDistance.Sign() <= 0 {
		return nil
	}

	return plan.MaxLoss.SetScale(riskScale).Div(plan.RiskDistance)
}

/*
scaled widens a decimal to the working precision, or reports absence.
*/
func scaled(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}

	return value.SetScale(riskScale)
}

/*
multiply scales a per-unit distance by a count of noise bands.
*/
func multiply(value *decimal.Decimal, factor float64) *decimal.Decimal {
	if value == nil || math.IsNaN(factor) || math.IsInf(factor, 0) || factor <= 0 {
		return nil
	}

	return value.SetScale(riskScale).Mul(
		decimal.NewFromFloat64(factor).SetScale(riskScale),
	)
}

/*
sum adds two optionally-absent amounts, treating absence as nothing contributed
rather than as zero, so a sum of nothing stays absent.
*/
func sum(left, right *decimal.Decimal) *decimal.Decimal {
	switch {
	case left == nil:
		return right
	case right == nil:
		return left
	default:
		return left.SetScale(riskScale).Add(right)
	}
}

/*
largest returns the greatest present value, or nil when every candidate is
absent.
*/
func largest(values ...*decimal.Decimal) *decimal.Decimal {
	var winner *decimal.Decimal

	for _, value := range values {
		if value == nil {
			continue
		}

		if winner == nil || value.Cmp(winner) > 0 {
			winner = value
		}
	}

	return winner
}

/*
ceilToTick rounds a price up onto the venue's quoting granularity. Lines a
position has to reach round up, so rounding never quietly lowers the bar.
*/
func ceilToTick(price, tick *decimal.Decimal) *decimal.Decimal {
	if price == nil {
		return nil
	}

	if tick == nil || tick.Sign() <= 0 {
		return price
	}

	ticks := decimal.ExactDivFloor(price, tick, 0)

	if ticks == nil {
		return price
	}

	rounded := ticks.SetScale(riskScale).Mul(tick)

	if rounded.Cmp(price) < 0 {
		rounded = rounded.Add(tick)
	}

	return rounded
}

/*
floorToTick rounds a price down onto the venue's quoting granularity. Floors
round down, so rounding never quietly tightens a boundary into an earlier exit.
*/
func floorToTick(price, tick *decimal.Decimal) *decimal.Decimal {
	if price == nil {
		return nil
	}

	if tick == nil || tick.Sign() <= 0 {
		return price
	}

	ticks := decimal.ExactDivFloor(price, tick, 0)

	if ticks == nil {
		return price
	}

	return ticks.SetScale(riskScale).Mul(tick)
}
