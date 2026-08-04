package types

import (
	"context"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
StopPhase is which question the regulator is currently answering.

The two phases exist because a single floor was being asked to do three
unrelated jobs at once: cap the loss, judge whether the thesis was still alive,
and protect a profit that had not been made yet. Only the first and third
belong here.
*/
type StopPhase string

const (
	/*
		PhaseDiscovery is an open lot that has not yet earned anything worth
		protecting. Peaks are recorded and the noise band is tracked, but the
		trailing floor is not allowed to end the trade — only the hard risk
		boundary is. Whether an underwater thesis is still viable is decided by
		the strategy's continuation evaluator, which can see the forecast, the
		causal evidence and the structural categories that answer it.
	*/
	PhaseDiscovery StopPhase = "discovery"
	/*
		PhaseProtected is a lot that has traded far enough above its profit line
		for the giveback floor to arm. From here the floor can only rise.
	*/
	PhaseProtected StopPhase = "protected"
)

/*
Stop trigger reasons. These are separate causes with separate urgencies, not
one "stopped out".
*/
const (
	// TriggerHardRisk is the maximum tolerable loss. No confirmation, no debounce.
	TriggerHardRisk = "hard_risk"
	// TriggerProtectedGiveback is a confirmed breach of a protected floor.
	TriggerProtectedGiveback = "protected_giveback"
)

/*
transitionHistory is how many geometry changes one lot keeps. It is enough to
reconstruct how a stop arrived at the floor it fired on without letting a
long-held position accumulate an unbounded audit trail in memory.
*/
const transitionHistory = 32

/*
StopTransition records one change in the regulator's geometry so an exit can be
explained after the fact, and so first-passage outcomes — did this reach its
profit line or its hard floor first — can be labelled later from the published
position rather than reconstructed from tick data.
*/
type StopTransition struct {
	At     time.Time        `json:"at"`
	Phase  StopPhase        `json:"phase"`
	Floor  *decimal.Decimal `json:"floor"`
	Mark   *decimal.Decimal `json:"mark"`
	Reason string           `json:"reason"`
}

/*
Fill is the realized economics of an entry crossing. Once the venue has
reported it there is nothing left to estimate on the entry side: the price paid
and the fee charged are facts, and only the exit crossing remains modeled.
*/
type Fill struct {
	EntryPrice *decimal.Decimal
	EntryFee   *decimal.Decimal
	Qty        *decimal.Decimal
}

/*
Stoploss regulates one open lot.

It owns exactly two exits. The hard floor is the maximum loss and fires on
sight. The protected floor is the profit the position has already earned and
fires only on a breach that holds. Everything else — forecast decay, structural
reversal, an ordinary drawdown that has not reached the boundary — belongs to
the strategy's continuation evaluator, which has the evidence to judge it.

Both Peak and Floor are monotonic. A peak once reached was reachable, and
protection once armed is not handed back because volatility later widened.
*/
type Stoploss struct {
	ctx    context.Context
	cancel context.CancelFunc

	Status Status    `json:"status"`
	Symbol string    `json:"symbol"`
	Phase  StopPhase `json:"phase"`

	/*
		Entry is the realized average price paid per unit, and Qty and EntryFee
		are the quantity and total fee it was paid on. Before the fill these
		hold the provisional estimate the order was placed against.
	*/
	Entry    *decimal.Decimal `json:"entry"`
	Qty      *decimal.Decimal `json:"qty"`
	EntryFee *decimal.Decimal `json:"entry_fee"`

	Mark *decimal.Decimal `json:"mark"`
	Peak *decimal.Decimal `json:"peak"`

	/*
		HardFloor is the loss boundary and is never lowered, never widened, and
		never made conditional on a model. ProfitLine is the executable price at
		which a sale covers entry cost, exit cost and the minimum edge.
		ArmLine is where protection turns on, ProfitFloor is where it locks, and
		TrailFloor follows the peak once it has.

		Floor is the highest of whichever of those are currently active, and is
		what an exit is actually measured against.
	*/
	HardFloor   *decimal.Decimal `json:"hard_floor"`
	ProfitLine  *decimal.Decimal `json:"profit_line"`
	ArmLine     *decimal.Decimal `json:"arm_line"`
	ProfitFloor *decimal.Decimal `json:"profit_floor"`
	TrailFloor  *decimal.Decimal `json:"trail_floor"`
	Floor       *decimal.Decimal `json:"floor"`

	ProfitArmed   bool   `json:"profit_armed"`
	TriggerReason string `json:"trigger_reason,omitempty"`

	Plan        RiskPlan         `json:"plan"`
	Transitions []StopTransition `json:"transitions,omitempty"`

	/*
		breachStreak counts consecutive executable marks at or below the
		protected floor. One wick through a floor is a print, not a decision.
	*/
	breachStreak int
}

/*
NewStoploss constructs a regulator for one lot from the geometry the entry was
sized under and whatever provisional basis is known before the fill.

The lines built here are estimates: the order has not crossed yet, so the entry
price is the ask the decision was priced at. RebindFill replaces them the
moment the venue says what was actually paid.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	entry *decimal.Decimal,
	qty *decimal.Decimal,
	entryFee *decimal.Decimal,
	mark *decimal.Decimal,
	plan RiskPlan,
) *Stoploss {
	errnie.Info("creating stoploss")

	ctx, cancel := context.WithCancel(ctx)

	stoploss := &Stoploss{
		ctx:      ctx,
		cancel:   cancel,
		Symbol:   symbol,
		Status:   PENDING,
		Phase:    PhaseDiscovery,
		Entry:    scaled(entry),
		Qty:      scaled(qty),
		EntryFee: scaled(entryFee),
		Mark:     scaled(mark),
		Plan:     plan,
	}

	stoploss.rebuild("armed")
	stoploss.Status = ARMED

	return stoploss
}

/*
RebindFill re-derives the geometry from what the entry crossing actually cost.

Until this runs the stop is defending an estimate. The venue reports a realized
average price, an accumulated fee and a filled quantity that can all differ
from what the order was priced at, and a break-even line built from the
estimate is wrong by exactly that difference — in the direction that makes the
position look more profitable than it is.

It is safe to call on every cumulative fill: the venue reports running totals,
so the last call wins.
*/
func (stoploss *Stoploss) RebindFill(fill Fill) {
	if stoploss == nil {
		return
	}

	if fill.EntryPrice != nil && fill.EntryPrice.Sign() > 0 {
		stoploss.Entry = scaled(fill.EntryPrice)
	}

	if fill.Qty != nil && fill.Qty.Sign() > 0 {
		stoploss.Qty = scaled(fill.Qty)
	}

	if fill.EntryFee != nil {
		stoploss.EntryFee = scaled(fill.EntryFee)
	}

	stoploss.rebuild("rebound_on_fill")
}

/*
Adopt installs geometry on a regulator that has none.

A lot recovered from the wallet is constructed before any ticker has priced its
symbol, so there is nothing to derive a boundary from at that moment. This is
how it gets one as soon as there is.

An existing plan is never replaced. The geometry an entry was sized under is the
contract that entry was made on, and quietly swapping it for one derived from a
later book would move the boundary the quantity was solved against.
*/
func (stoploss *Stoploss) Adopt(plan RiskPlan) {
	if stoploss == nil || !plan.Present || stoploss.Plan.Present {
		return
	}

	stoploss.Plan = plan
	stoploss.rebuild("adopted_geometry")
}

/*
Observe advances the state machine from one executable mark.

The order of operations is the whole point. A peak is recorded before anything
is allowed to exit, so a legitimate high is never lost to the tick that follows
it; arming is decided separately from recording, so a position can build a peak
while still underwater; and the floor is raised before it is tested, so it can
only ever have moved upward.
*/
func (stoploss *Stoploss) Observe(evidence StopEvidence) {
	if stoploss == nil || stoploss.Status == TRIGGERED {
		return
	}

	mark := scaled(evidence.ExecutableMark)

	if mark == nil || mark.Sign() <= 0 {
		return
	}

	stoploss.Mark = mark
	stoploss.Plan = stoploss.Plan.Refresh(evidence.Spread, evidence.Impact)

	// A retreating quote is a price nothing could have been sold into, so it
	// records no peak — but it is still a mark, and the floors below still
	// judge it.
	if evidence.GeometryValid() && (stoploss.Peak == nil || mark.Cmp(stoploss.Peak) > 0) {
		stoploss.Peak = mark.Copy()
		stoploss.TrailFloor = floorToTick(
			subtract(stoploss.Peak, stoploss.Plan.TrailDistance),
			stoploss.Plan.TickSize,
		)
	}

	if !stoploss.ProfitArmed && stoploss.ArmLine != nil &&
		mark.Cmp(stoploss.ArmLine) >= 0 {
		stoploss.ProfitArmed = true
		stoploss.Phase = PhaseProtected
		stoploss.record("profit_armed")
	}

	stoploss.raiseFloor()

	switch {
	case stoploss.HardFloor != nil && mark.Cmp(stoploss.HardFloor) <= 0:
		stoploss.trigger(TriggerHardRisk)
	case stoploss.ProfitArmed && stoploss.confirmedBreach(mark):
		stoploss.trigger(TriggerProtectedGiveback)
	}
}

/*
raiseFloor recomputes the active floor and installs it only if it is higher
than the one already standing.

The trailing floor is gated on profit protection rather than applied from the
first tick. In discovery the position has not earned anything to give back, and
a percentage trail on an entry price is just a second, tighter loss boundary
wearing the language of profit.
*/
func (stoploss *Stoploss) raiseFloor() {
	candidate := stoploss.HardFloor

	if stoploss.ProfitArmed {
		candidate = largest(candidate, stoploss.ProfitFloor, stoploss.TrailFloor)
	}

	raised := largest(stoploss.Floor, candidate)

	if raised == nil {
		return
	}

	if stoploss.Floor == nil || raised.Cmp(stoploss.Floor) != 0 {
		stoploss.Floor = raised
		stoploss.record("floor_raised")
	}
}

/*
confirmedBreach reports whether the protected floor has been breached by
enough consecutive marks to be believed.

A single observation is not enough. The price path this exists for is full of
long individual wicks, and exiting on the first print below a floor hands the
position to whoever printed it.
*/
func (stoploss *Stoploss) confirmedBreach(mark *decimal.Decimal) bool {
	if stoploss.Floor == nil {
		return false
	}

	if mark.Cmp(stoploss.Floor) > 0 {
		stoploss.breachStreak = 0
		return false
	}

	stoploss.breachStreak++

	return stoploss.breachStreak >= max(1, stoploss.Plan.ConfirmMarks)
}

/*
rebuild derives every line from the current basis and plan.

The profit line is solved from what liquidating actually yields rather than by
adding fees onto the entry price. Fee amounts are charged on the proceeds of
the sale, so the exit fee divides out of the price the sale has to reach; the
entry fee is a total that becomes a per-unit contribution only after the
quantity it was charged on is known.
*/
func (stoploss *Stoploss) rebuild(reason string) {
	plan := stoploss.Plan

	if !plan.Present || stoploss.Entry == nil || stoploss.Entry.Sign() <= 0 {
		return
	}

	tick := plan.TickSize

	stoploss.HardFloor = floorToTick(
		subtract(stoploss.Entry, plan.RiskDistance), tick,
	)

	stoploss.ProfitLine = ceilToTick(stoploss.profitLine(), tick)

	if stoploss.ProfitLine != nil {
		stoploss.ArmLine = ceilToTick(
			sum(stoploss.ProfitLine, plan.ArmBuffer), tick,
		)
		stoploss.ProfitFloor = floorToTick(
			sum(stoploss.ProfitLine, plan.LockBuffer), tick,
		)
	}

	if stoploss.Peak != nil {
		stoploss.TrailFloor = floorToTick(
			subtract(stoploss.Peak, plan.TrailDistance), tick,
		)
	}

	stoploss.raiseFloor()
	stoploss.record(reason)
}

/*
profitLine solves for the executable price at which selling the whole position
clears its entry cost, its exit fee and the minimum edge.

	price = (entry×qty + entryFee + minEdge×qty) / (qty × (1 − exitFeeRate))

Depth impact is deliberately absent from the numerator: the mark this line is
compared against is already the impact-adjusted sell VWAP for the position's
quantity, and pricing the same friction in both places would demand a profit
the position has to earn twice.
*/
func (stoploss *Stoploss) profitLine() *decimal.Decimal {
	quantity := stoploss.Qty

	if quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	cost := stoploss.Entry.Mul(quantity)

	if stoploss.EntryFee != nil {
		cost = cost.Add(stoploss.EntryFee)
	}

	if stoploss.Plan.MinEdge != nil {
		cost = cost.Add(stoploss.Plan.MinEdge.SetScale(riskScale).Mul(quantity))
	}

	proceeds := quantity.Copy()

	if rate := stoploss.Plan.ExitFeeRate; rate != nil &&
		rate.Sign() > 0 && rate.Cmp(decimal.NewFromInt64(1).SetScale(riskScale)) < 0 {
		proceeds = quantity.Mul(
			decimal.NewFromInt64(1).SetScale(riskScale).Sub(rate),
		)
	}

	if proceeds.Sign() <= 0 {
		return nil
	}

	return cost.Div(proceeds)
}

/*
trigger latches the exit and the cause that produced it. The regulator is
one-shot: once it has fired, later marks change nothing, because the sell is
already on its way to the venue.
*/
func (stoploss *Stoploss) trigger(reason string) {
	stoploss.Status = TRIGGERED
	stoploss.TriggerReason = reason
	stoploss.record(reason)
}

/*
record appends one geometry change to the bounded audit trail.
*/
func (stoploss *Stoploss) record(reason string) {
	transition := StopTransition{
		At:     time.Now().UTC(),
		Phase:  stoploss.Phase,
		Reason: reason,
	}

	if stoploss.Floor != nil {
		transition.Floor = stoploss.Floor.Copy()
	}

	if stoploss.Mark != nil {
		transition.Mark = stoploss.Mark.Copy()
	}

	stoploss.Transitions = append(stoploss.Transitions, transition)

	if len(stoploss.Transitions) > transitionHistory {
		stoploss.Transitions = stoploss.Transitions[len(stoploss.Transitions)-transitionHistory:]
	}
}

/*
Close cancels the regulator context.
*/
func (stoploss *Stoploss) Close() (err error) {
	if stoploss.cancel != nil {
		stoploss.cancel()
	}

	return err
}

/*
subtract removes an optionally-absent distance from a price, treating absence
as no distance to apply rather than as zero.
*/
func subtract(price, distance *decimal.Decimal) *decimal.Decimal {
	if price == nil || distance == nil {
		return price
	}

	return price.SetScale(riskScale).Sub(distance)
}
