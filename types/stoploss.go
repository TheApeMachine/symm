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
	// PhaseDiscovery is an open lot that has not yet earned anything worth
	// protecting. Peaks are recorded and the noise band is tracked, but the
	// trailing floor is not allowed to end the trade — only the hard risk
	// boundary or a live regime circuit breaker may. Ordinary forecast revisions
	// remain diagnostic while the position is open.
	PhaseDiscovery StopPhase = "discovery"
	// PhaseProtected is a lot that has traded far enough above its profit line
	// for the giveback floor to arm. From here the floor can only rise.
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
	// TriggerProfitFailSafe is the last line above break-even on a lot that had
	// already earned a profit. Immediate, like the hard floor.
	TriggerProfitFailSafe = "profit_failsafe"
	// TriggerRecoveredExit adopts sell authority already working at the venue
	// when the process restarts.
	TriggerRecoveredExit = "recovered_working_exit"
	// TriggerPumpDumpSellIgnition is an empirically scaled downward ignition.
	TriggerPumpDumpSellIgnition = "pumpdump_sell_ignition"
	// TriggerHawkesSellCascade is a self-exciting sell process expected to
	// produce at least one descendant per sell parent.
	TriggerHawkesSellCascade = "hawkes_sell_cascade"
	// TriggerExecutionNoiseRegime fires when one live crossing band exceeds
	// the configured number of entry bands used to place the hard floor.
	TriggerExecutionNoiseRegime = "execution_noise_regime"
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
	/*
		Seq orders transitions across the whole life of one regulator, including
		the ones the bounded history has already dropped. It is what lets a
		consumer drain only what it has not seen without holding an index into a
		slice that shifts underneath it.
	*/
	Seq            uint64           `json:"seq"`
	At             time.Time        `json:"at"`
	Phase          StopPhase        `json:"phase"`
	Status         Status           `json:"status"`
	Reason         string           `json:"reason"`
	TriggerReason  string           `json:"trigger_reason,omitempty"`
	ProfitArmed    bool             `json:"profit_armed"`
	Mark           *decimal.Decimal `json:"mark"`
	Peak           *decimal.Decimal `json:"peak"`
	Floor          *decimal.Decimal `json:"floor"`
	HardFloor      *decimal.Decimal `json:"hard_floor"`
	ProfitLine     *decimal.Decimal `json:"profit_line"`
	BreakEvenLine  *decimal.Decimal `json:"break_even_line"`
	ProfitFailSafe *decimal.Decimal `json:"profit_failsafe"`
	ProfitFloor    *decimal.Decimal `json:"profit_floor"`
	TrailFloor     *decimal.Decimal `json:"trail_floor"`
	ArmLine        *decimal.Decimal `json:"arm_line"`
	Entry          *decimal.Decimal `json:"entry"`
	Qty            *decimal.Decimal `json:"qty"`
	EntryFee       *decimal.Decimal `json:"entry_fee"`
	RiskDistance   *decimal.Decimal `json:"risk_distance"`
	TrailDistance  *decimal.Decimal `json:"trail_distance"`
	EntryNoiseBand *decimal.Decimal `json:"entry_noise_band"`
	NoiseBand      *decimal.Decimal `json:"noise_band"`
	WorstCaseLoss  *decimal.Decimal `json:"worst_case_loss"`
	MaxAdverse     *decimal.Decimal `json:"max_adverse"`
	MaxFavorable   *decimal.Decimal `json:"max_favorable"`
	BasisConfirmed bool             `json:"basis_confirmed"`
	DepthLimited   bool             `json:"depth_limited"`
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

It owns two price exits and two regime circuit breakers. The hard floor is the
maximum loss and fires on sight. The protected floor is the profit the position
has already earned and fires only on a breach that holds. A validated structural
break or an execution-noise band that has outgrown the entry regime exits before
either price line; ordinary forecast decay and drawdown remain diagnostic.

Both Peak and Floor are monotonic. A peak once reached was reachable, and
protection once armed is not handed back because volatility later widened.
*/
type Stoploss struct {
	ctx    context.Context
	cancel context.CancelFunc

	Status Status    `json:"status"`
	Symbol string    `json:"symbol"`
	Phase  StopPhase `json:"phase"`

	// Entry is the realized average price paid per unit, and Qty and EntryFee
	// are the quantity and total fee it was paid on. Before the fill these
	// hold the provisional estimate the order was placed against.
	Entry    *decimal.Decimal `json:"entry"`
	Qty      *decimal.Decimal `json:"qty"`
	EntryFee *decimal.Decimal `json:"entry_fee"`

	Mark *decimal.Decimal `json:"mark"`
	Peak *decimal.Decimal `json:"peak"`

	// HardFloor is the loss boundary and is never lowered, never widened, and
	// never made conditional on a model. ProfitLine is the executable price at
	// which a sale covers entry cost, exit cost and the minimum edge.
	// ArmLine is where protection turns on, ProfitFloor is where it locks, and
	// TrailFloor follows the peak once it has.
	//
	// Floor is the highest of whichever of those are currently active, and is
	// what an exit is actually measured against.
	HardFloor  *decimal.Decimal `json:"hard_floor"`
	ProfitLine *decimal.Decimal `json:"profit_line"`
	/*
		BreakEvenLine is where a sale exactly covers what the lot cost, with no
		edge on top. It is stated separately from ProfitLine because the two
		answer different questions: ProfitLine is where the trade becomes worth
		having, BreakEvenLine is where it stops being worth keeping.
	*/
	BreakEvenLine *decimal.Decimal `json:"break_even_line"`
	/*
		ProfitFailSafe sits between the two, one execution reserve above
		break-even. It is the floor a lot that has already earned something is
		not allowed to fall through while a confirmation count runs.
	*/
	ProfitFailSafe *decimal.Decimal `json:"profit_failsafe"`
	ArmLine        *decimal.Decimal `json:"arm_line"`
	ProfitFloor    *decimal.Decimal `json:"profit_floor"`
	TrailFloor     *decimal.Decimal `json:"trail_floor"`
	Floor          *decimal.Decimal `json:"floor"`

	ProfitArmed   bool             `json:"profit_armed"`
	TriggerReason string           `json:"trigger_reason,omitempty"`
	DepthLimited  bool             `json:"depth_limited"`
	MaxAdverse    *decimal.Decimal `json:"max_adverse"`
	MaxFavorable  *decimal.Decimal `json:"max_favorable"`
	/*
		BasisConfirmed is false until the venue has said what this lot actually
		cost. Before then the regulator has lines drawn from the ask the order
		was priced at, and they are for display and sizing only — no mark may
		record a peak, arm protection, or trigger against an estimate.

		The hole this closes is narrow and expensive. A market buy that takes a
		few seconds to fill sees ticks the whole time; without this gate those
		ticks build a peak, and a peak two noise bands above an unfilled order
		can cross the arm line. The lot then opens already protecting a profit
		that belonged to a price it never paid, and gives it back on the first
		confirmed breach.
	*/
	BasisConfirmed bool `json:"basis_confirmed"`

	Plan        RiskPlan         `json:"plan"`
	Transitions []StopTransition `json:"transitions,omitempty"`

	// breachStreak counts consecutive executable marks at or below the
	// protected floor. One wick through a floor is a print, not a decision.
	breachStreak int
	// sequence numbers every transition ever recorded and emitted marks how far
	// a consumer has drained. They are kept separate from the Transitions slice
	// because that slice is bounded: a plain index into it would re-emit rows
	// as soon as trimming shifted them.
	sequence uint64
	emitted  uint64
}

/*
StopSnapshot is a value copy of one regulator's geometry, taken on the goroutine
that owns it so another can read it without touching the live struct.

It exists because the strategy has to price the position's remaining upside and
downside against the same boundaries the regulator is defending, and reaching
into the regulator to read them would put two goroutines on it.
*/
type StopSnapshot struct {
	Present        bool             `json:"present"`
	Symbol         string           `json:"symbol"`
	Phase          StopPhase        `json:"phase"`
	Status         Status           `json:"status"`
	TriggerReason  string           `json:"trigger_reason,omitempty"`
	ProfitArmed    bool             `json:"profit_armed"`
	BasisConfirmed bool             `json:"basis_confirmed"`
	DepthLimited   bool             `json:"depth_limited"`
	Entry          *decimal.Decimal `json:"entry"`
	Mark           *decimal.Decimal `json:"mark"`
	Peak           *decimal.Decimal `json:"peak"`
	HardFloor      *decimal.Decimal `json:"hard_floor"`
	ProfitLine     *decimal.Decimal `json:"profit_line"`
	BreakEvenLine  *decimal.Decimal `json:"break_even_line"`
	ProfitFailSafe *decimal.Decimal `json:"profit_failsafe"`
	ProfitFloor    *decimal.Decimal `json:"profit_floor"`
	ArmLine        *decimal.Decimal `json:"arm_line"`
	RiskDistance   *decimal.Decimal `json:"risk_distance"`
	EntryNoiseBand *decimal.Decimal `json:"entry_noise_band"`
	NoiseBand      *decimal.Decimal `json:"noise_band"`
	WorstCaseLoss  *decimal.Decimal `json:"worst_case_loss"`
	MaxAdverse     *decimal.Decimal `json:"max_adverse"`
	MaxFavorable   *decimal.Decimal `json:"max_favorable"`
}

/*
Snapshot copies the regulator's current geometry.

Absent geometry yields Present false rather than zeroes, because a hard floor of
zero reads as "no risk" and a lot whose boundary has not been derived yet must
not be scored as though it had one.
*/
func (stoploss *Stoploss) Snapshot() StopSnapshot {
	if stoploss == nil {
		return StopSnapshot{}
	}

	return StopSnapshot{
		Present:        stoploss.Plan.Present,
		Symbol:         stoploss.Symbol,
		Phase:          stoploss.Phase,
		Status:         stoploss.Status,
		TriggerReason:  stoploss.TriggerReason,
		ProfitArmed:    stoploss.ProfitArmed,
		BasisConfirmed: stoploss.BasisConfirmed,
		DepthLimited:   stoploss.DepthLimited,
		Entry:          copyDecimal(stoploss.Entry),
		Mark:           copyDecimal(stoploss.Mark),
		Peak:           copyDecimal(stoploss.Peak),
		HardFloor:      copyDecimal(stoploss.HardFloor),
		ProfitLine:     copyDecimal(stoploss.ProfitLine),
		BreakEvenLine:  copyDecimal(stoploss.BreakEvenLine),
		ProfitFailSafe: copyDecimal(stoploss.ProfitFailSafe),
		ProfitFloor:    copyDecimal(stoploss.ProfitFloor),
		ArmLine:        copyDecimal(stoploss.ArmLine),
		RiskDistance:   copyDecimal(stoploss.Plan.RiskDistance),
		EntryNoiseBand: copyDecimal(stoploss.Plan.EntryNoiseBand),
		NoiseBand:      copyDecimal(stoploss.Plan.NoiseBand),
		WorstCaseLoss:  stoploss.worstCaseLoss(),
		MaxAdverse:     copyDecimal(stoploss.MaxAdverse),
		MaxFavorable:   copyDecimal(stoploss.MaxFavorable),
	}
}

/*
copyDecimal copies an optionally-absent amount, keeping absence absent so the
reader cannot mistake "not derived" for zero.
*/
func copyDecimal(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}

	return value
}

/*
DrainTransitions returns the geometry changes that have not been handed to a
consumer yet, and marks them taken.

The regulator keeps its recent history for anyone reading the published
position, but an audit trail has to be written exactly once. Draining by
sequence rather than by slice position means a burst that overflows the bounded
history loses the oldest rows instead of replaying the newest.
*/
func (stoploss *Stoploss) DrainTransitions() []StopTransition {
	if stoploss == nil || stoploss.emitted >= stoploss.sequence {
		return nil
	}

	pending := make([]StopTransition, 0, stoploss.sequence-stoploss.emitted)

	for _, transition := range stoploss.Transitions {
		if transition.Seq > stoploss.emitted {
			pending = append(pending, transition)
		}
	}

	stoploss.emitted = stoploss.sequence

	return pending
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
		ctx:          ctx,
		cancel:       cancel,
		Symbol:       symbol,
		Status:       PENDING,
		Phase:        PhaseDiscovery,
		Entry:        scaled(entry),
		Qty:          scaled(qty),
		EntryFee:     scaled(entryFee),
		Mark:         scaled(mark),
		Plan:         plan,
		MaxAdverse:   decimal.NewFromInt64(0).SetScale(riskScale),
		MaxFavorable: decimal.NewFromInt64(0).SetScale(riskScale),
	}

	stoploss.rebuild("provisional")
	stoploss.Status = PENDING

	return stoploss
}

/*
BindRecovered adopts a basis that is already real without waiting for a fill.

Wallet inventory was bought at some point in the past and the trade history says
what it cost, so there is no estimate to replace and nothing to wait for. This
exists as a separate door from RebindFill so that the ordinary path cannot be
talked into confirming a basis the venue never reported.
*/
func (stoploss *Stoploss) BindRecovered() {
	if stoploss == nil || stoploss.BasisConfirmed {
		return
	}

	stoploss.confirmBasis("bound_recovered")
}

/*
BindRecoveredExit adopts a working venue sell after its inventory basis has
been recovered.
*/
func (stoploss *Stoploss) BindRecoveredExit() {
	if stoploss == nil || !stoploss.BasisConfirmed || stoploss.Status == TRIGGERED {
		return
	}

	stoploss.trigger(TriggerRecoveredExit)
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

	if stoploss.Status == TRIGGERED {
		stoploss.rebuild("rebound_during_exit")
		return
	}

	reason := "rebound_on_fill"

	if !stoploss.BasisConfirmed {
		reason = "bound_on_fill"
	}

	stoploss.confirmBasis(reason)
}

/*
confirmBasis promotes the regulator from an estimate to a real basis and throws
away everything it thought it knew on the way there.

The discarded state is the point. A peak, an armed flag or a floor established
while the lot was still unfilled describes a price path the position was never
in, and rebuilding the lines around a corrected entry while keeping that path
would leave the contradiction in place — protection armed against a peak that
predates ownership.
*/
func (stoploss *Stoploss) confirmBasis(reason string) {
	stoploss.Peak = nil
	stoploss.TrailFloor = nil
	stoploss.Floor = nil
	stoploss.ProfitArmed = false
	stoploss.Phase = PhaseDiscovery
	stoploss.TriggerReason = ""
	stoploss.DepthLimited = false
	stoploss.MaxAdverse = decimal.NewFromInt64(0).SetScale(riskScale)
	stoploss.MaxFavorable = decimal.NewFromInt64(0).SetScale(riskScale)
	stoploss.breachStreak = 0
	stoploss.BasisConfirmed = true
	stoploss.Status = ARMED

	stoploss.rebuild(reason)
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
	if stoploss == nil || stoploss.Status == TRIGGERED || !stoploss.BasisConfirmed {
		return
	}

	mark := scaled(evidence.ExecutableMark)

	if mark == nil || mark.Sign() <= 0 {
		return
	}

	/*
		When the visible book cannot absorb the whole position the mark is the
		touch price — real, quoted, and true only of the part that fits. What
		follows treats that asymmetrically on purpose.

		Profit geometry is refused: a peak or an arming taken from a price only
		a fraction of the lot could reach would protect a profit the position
		could not realise. The hard floor is not refused, because a book too
		thin to absorb the lot is evidence of more danger, not less, and
		suspending the loss boundary exactly when liquidity is disappearing is
		the opposite of what it is for.
	*/
	trustGeometry := !evidence.DepthLimited
	now := time.Now().UTC()
	noiseRegimeBroken := false

	stoploss.Mark = mark
	stoploss.DepthLimited = evidence.DepthLimited

	if evidence.Fresh(now) {
		stoploss.Plan = stoploss.Plan.Refresh(evidence.Spread, evidence.Impact)
		entryNoiseLimit := multiply(
			stoploss.Plan.EntryNoiseBand,
			stoploss.Plan.Multiples.Risk,
		)
		noiseRegimeBroken = entryNoiseLimit != nil &&
			stoploss.Plan.NoiseBand != nil &&
			stoploss.Plan.NoiseBand.Cmp(entryNoiseLimit) > 0
	}

	stoploss.observeExcursion(mark, trustGeometry)

	if evidence.RegimeExit != "" && evidence.Fresh(now) {
		stoploss.trigger(evidence.RegimeExit)
		return
	}

	if noiseRegimeBroken {
		stoploss.trigger(TriggerExecutionNoiseRegime)
		return
	}

	// A hollow quote is a price nothing could have been sold into, so it
	// records no peak — but it is still a mark, and the floors below still
	// judge it.
	if trustGeometry && evidence.GeometryValidAt(now) &&
		(stoploss.Peak == nil || mark.Cmp(stoploss.Peak) > 0) {
		stoploss.Peak = mark
		stoploss.TrailFloor = floorToTick(
			subtract(stoploss.Peak, stoploss.Plan.TrailDistance),
			stoploss.Plan.TickSize,
		)
	}

	if trustGeometry && !stoploss.ProfitArmed && stoploss.ArmLine != nil &&
		mark.Cmp(stoploss.ArmLine) >= 0 {
		stoploss.ProfitArmed = true
		stoploss.Phase = PhaseProtected
		stoploss.record("profit_armed")
	}

	stoploss.raiseFloor()

	switch {
	case stoploss.HardFloor != nil && mark.Cmp(stoploss.HardFloor) <= 0:
		stoploss.trigger(TriggerHardRisk)
	/*
		The fail-safe sits below the giveback floor and above break-even, and it
		does not wait for confirmation.

		Confirmation is what stops a single wick from ending a protected trade,
		and it necessarily costs something: three marks below the floor is three
		marks of giveback, and on a fast reversal that path can carry the
		position back through the price at which the round trip stops paying
		for itself. Debouncing the upper line and leaving the lower one
		immediate keeps the wick tolerance without letting it spend the profit
		it was protecting.
	*/
	case stoploss.ProfitArmed && stoploss.ProfitFailSafe != nil &&
		mark.Cmp(stoploss.ProfitFailSafe) <= 0:
		stoploss.trigger(TriggerProfitFailSafe)
	case stoploss.ProfitArmed && stoploss.confirmedBreach(mark):
		stoploss.trigger(TriggerProtectedGiveback)
	}
}

/*
observeExcursion records the broker-cadence path in risk-distance units.
Depth-limited adverse marks remain valid; favorable ones cannot prove that the
whole position could have sold at the touch.
*/
func (stoploss *Stoploss) observeExcursion(mark *decimal.Decimal, trusted bool) {
	if stoploss.Entry == nil || stoploss.Plan.RiskDistance == nil ||
		stoploss.Plan.RiskDistance.Sign() <= 0 {
		return
	}

	excursion := mark.SetScale(riskScale).
		Sub(stoploss.Entry).
		Div(stoploss.Plan.RiskDistance)

	if stoploss.MaxAdverse == nil || excursion.Cmp(stoploss.MaxAdverse) < 0 {
		stoploss.MaxAdverse = excursion
	}

	if trusted && (stoploss.MaxFavorable == nil ||
		excursion.Cmp(stoploss.MaxFavorable) > 0) {
		stoploss.MaxFavorable = excursion
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

	stoploss.BreakEvenLine = ceilToTick(stoploss.liquidationLine(nil), tick)
	stoploss.ProfitLine = ceilToTick(
		stoploss.liquidationLine(plan.MinEdge), tick,
	)

	if stoploss.ProfitLine != nil {
		stoploss.ArmLine = ceilToTick(
			sum(stoploss.ProfitLine, plan.ArmBuffer), tick,
		)
		stoploss.ProfitFloor = floorToTick(
			sum(stoploss.ProfitLine, plan.LockBuffer), tick,
		)
	}

	/*
		The fail-safe is one lock buffer above break-even rather than a share of
		the profit line, so it stays anchored to the price the round trip stops
		paying at no matter how much edge the lot went on to earn.
	*/
	if stoploss.BreakEvenLine != nil {
		stoploss.ProfitFailSafe = ceilToTick(
			sum(stoploss.BreakEvenLine, plan.LockBuffer), tick,
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
liquidationLine solves for the executable price at which selling the whole
position clears its entry cost, its exit fee, and whatever edge is demanded on
top.

	price = (entry×qty + entryFee + edge×qty) / (qty × (1 − exitFeeRate))

An absent edge gives the break-even line: exactly what the round trip cost, and
not a cent more. Passing MinEdge gives the profit line. They are the same
arithmetic because they are the same question asked with a different bar, and
computing them separately is how the two drift apart.

Depth impact is deliberately absent from the numerator: the mark these lines are
compared against is already adjusted for what the position's own size costs to
liquidate, and pricing the same friction in both places would demand a profit
the position has to earn twice.
*/
func (stoploss *Stoploss) liquidationLine(edge *decimal.Decimal) *decimal.Decimal {
	quantity := stoploss.Qty

	if quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	cost := stoploss.Entry.Mul(quantity)

	if stoploss.EntryFee != nil {
		cost = cost.Add(stoploss.EntryFee)
	}

	if edge != nil {
		cost = cost.Add(edge.SetScale(riskScale).Mul(quantity))
	}

	proceeds := quantity

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
worstCaseLoss returns the quote-currency loss at the hard floor, including the
entry fee already paid and the exit fee charged on floor proceeds.
*/
func (stoploss *Stoploss) worstCaseLoss() *decimal.Decimal {
	if stoploss.Entry == nil || stoploss.Qty == nil || stoploss.Qty.Sign() <= 0 ||
		stoploss.HardFloor == nil {
		return nil
	}

	cost := stoploss.Entry.SetScale(riskScale).Mul(stoploss.Qty)

	if stoploss.EntryFee != nil {
		cost = cost.Add(stoploss.EntryFee)
	}

	proceeds := stoploss.HardFloor.SetScale(riskScale).Mul(stoploss.Qty)

	if rate := stoploss.Plan.ExitFeeRate; rate != nil && rate.Sign() > 0 {
		proceeds = proceeds.Mul(
			decimal.NewFromInt64(1).SetScale(riskScale).Sub(rate),
		)
	}

	loss := cost.Sub(proceeds)

	if loss.Sign() < 0 {
		return decimal.NewFromInt64(0).SetScale(riskScale)
	}

	return loss
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
	stoploss.sequence++

	transition := StopTransition{
		Seq:            stoploss.sequence,
		At:             time.Now().UTC(),
		Phase:          stoploss.Phase,
		Status:         stoploss.Status,
		Reason:         reason,
		TriggerReason:  stoploss.TriggerReason,
		ProfitArmed:    stoploss.ProfitArmed,
		Mark:           copyDecimal(stoploss.Mark),
		Peak:           copyDecimal(stoploss.Peak),
		Floor:          copyDecimal(stoploss.Floor),
		HardFloor:      copyDecimal(stoploss.HardFloor),
		ProfitLine:     copyDecimal(stoploss.ProfitLine),
		BreakEvenLine:  copyDecimal(stoploss.BreakEvenLine),
		ProfitFailSafe: copyDecimal(stoploss.ProfitFailSafe),
		ProfitFloor:    copyDecimal(stoploss.ProfitFloor),
		TrailFloor:     copyDecimal(stoploss.TrailFloor),
		ArmLine:        copyDecimal(stoploss.ArmLine),
		Entry:          copyDecimal(stoploss.Entry),
		Qty:            copyDecimal(stoploss.Qty),
		EntryFee:       copyDecimal(stoploss.EntryFee),
		RiskDistance:   copyDecimal(stoploss.Plan.RiskDistance),
		TrailDistance:  copyDecimal(stoploss.Plan.TrailDistance),
		EntryNoiseBand: copyDecimal(stoploss.Plan.EntryNoiseBand),
		NoiseBand:      copyDecimal(stoploss.Plan.NoiseBand),
		WorstCaseLoss:  stoploss.worstCaseLoss(),
		MaxAdverse:     copyDecimal(stoploss.MaxAdverse),
		MaxFavorable:   copyDecimal(stoploss.MaxFavorable),
		BasisConfirmed: stoploss.BasisConfirmed,
		DepthLimited:   stoploss.DepthLimited,
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
