package strategy

import (
	"math"
	"slices"
	"time"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/system"
)

/*
FrameDelimiter is the reserved delimiter token marking temporal state transitions
in precursor context paths. Bit 53 is distinct from ConditionToken (bit 52) and
quantity identities (bits 4..51).
*/
const FrameDelimiter uint64 = 1 << 53

/*
learningMarket owns independent per-symbol virtual wallets and the evolving
temporal precursor history across distinct Impulse state changes.
*/
type learningMarket struct {
	symbol            string
	status            string
	regions           []learning.Region
	currentConditions []uint64
	history           [][]uint64
	authority         float64

	/*
		epochs measures this instrument's own cadence of impulse change: the
		mean interval between impulse states that actually moved. It reports
		how fast this market's observed state turns over; it does not set the
		measurement window, because how often a feature state changes says
		nothing about how long a price move takes to cover a cost.
	*/
	epochAt   time.Time
	epochMean float64
	epochs    uint64

	/*
		exploration rotates which feasible action each counterfactual lane
		reaches for.

		There are four of them and usually more feasible actions than that, so a
		fixed lane-to-action assignment explores only the first four of the list
		and never the rest. That is not a thin spot in the evidence, it is a
		permanent hole: the deeper size bisections and every reduction would
		carry zero samples for the life of the run, and an action with no
		evidence has no measured support for the policy to prefer — so the
		actions nobody explores stay permanently unmeasurable.
	*/
	exploration uint64

	/*
		movement is this instrument's own dispersion of log midpoint return per
		observation, and observationMean is how much wall time one observation
		actually spans. Together with the round trip a decision pays, they give
		the window over which that decision becomes answerable.
	*/
	movement        *adaptive.Baseline
	lastMid         float64
	observationAt   time.Time
	observationMean float64
	observations    uint64
	sigma           float64
	hasSigma        bool
	cost            float64
	lanes           []learningLane
	context         []uint64
	actions         []LearningAction
	events          []hindsight.LearningEvent
	at              time.Time
	seq             hindsight.CaptureSequence
	capture         hindsight.CaptureIdentity
	gridVersion     uint64

	// exposure is the policy lane's inventory history, used to judge episodes
	// the delay line confirms after the fact.
	exposure []exposureSpan

	/*
		observedFrom is when this market began watching its own inventory.

		A lane that has never taken a position leaves no spans behind, and that
		emptiness is not ignorance: from this point onward the desk demonstrably
		held nothing, so every episode inside the watched window was sat out.
		Without this the two are indistinguishable, and a policy that took no
		position at all reports every move it missed as unknowable.
	*/
	observedFromSeq hindsight.CaptureSequence
	observedFrom    time.Time
	observing       bool

	/*
		trail is what the agent actually saw, kept so an episode confirmed
		later can be trained against the state that preceded it.

		It has to be retained rather than reconstructed. Grid regions are read
		from running baseline estimators, so the context at a past coordinate
		is not recoverable from that coordinate's stored metrics — only the
		agent, standing there at the time, ever held it.
	*/
	trail []observedContext
}

/*
observedContext is one moment the agent conditioned on, addressed by the
capture coordinate that produced it. The context already carries this market's
recent past — current impulse, then the frames before it — so a single retained
entry is the whole "what led here", not just an instant.
*/
type observedContext struct {
	seq          hindsight.CaptureSequence
	at           time.Time
	accountState string
	context      []uint64
	mid          float64
}

/*
observeContext retains the context the agent is conditioning on now. Only
changes are stored: a market sitting in one impulse state across a thousand
book updates is one entry, not a thousand.
*/
func (market *learningMarket) observeContext(context []uint64, accountState string) {
	if len(context) == 0 {
		return
	}
	last := len(market.trail) - 1

	if last >= 0 && market.trail[last].accountState == accountState &&
		slices.Equal(market.trail[last].context, context) {
		return
	}

	market.trail = append(market.trail, observedContext{
		seq: market.seq, at: market.at, accountState: accountState,
		context: append([]uint64(nil), context...), mid: market.lastMid,
	})

	if len(market.trail) > system.Cfg.Learning.RetainedContexts {
		market.trail = append(market.trail[:0], market.trail[1:]...)
	}
}

/*
contextAt answers what the agent was conditioning on at a capture coordinate:
the most recent retained context at or before it. A coordinate older than the
retained trail returns nothing rather than the oldest entry, because the honest
answer there is that this is no longer known.
*/
func (market *learningMarket) contextAt(seq hindsight.CaptureSequence) (observedContext, bool) {
	found := observedContext{}
	ok := false

	for _, entry := range market.trail {
		if entry.seq > seq {
			break
		}
		found, ok = entry, true
	}

	if ok && len(market.trail) > 0 && market.trail[0].seq > seq {
		return observedContext{}, false
	}

	return found, ok
}

/*
epoch folds one observed interval between impulse changes into the mean. It is
the instrument's own clock: everything measured in epochs adapts to how fast
this particular market actually moves.
*/
func (market *learningMarket) epoch(at time.Time) {
	if !market.epochAt.IsZero() && at.After(market.epochAt) {
		market.epochs++
		market.epochMean += (at.Sub(market.epochAt).Seconds() - market.epochMean) / float64(market.epochs)
	}

	market.epochAt = at
}

/*
movementReadiness is how many observations a dispersion needs behind it before
the window may be derived from it. A spread computed from one or two readings
is not a dispersion, and an underestimate here shortens the window — the exact
direction that makes acting look uniformly worse than waiting. An overestimate
only lengthens it, which the attribution ceiling already bounds, so the gate is
deliberately asymmetric in the safe direction.
*/
const movementReadiness = 32

/*
observe folds one book update into this instrument's own measures of movement
and cadence: the dispersion of its log midpoint return per observation, and how
much wall time one observation actually spans. cost is the round trip that
opening and closing a position here would pay, as a fraction of price.

The dispersion is carried by an adaptive baseline rather than a running total,
so it forgets: an instrument that was volatile an hour ago and is quiet now must
be measured on the market it is in, not the one it was in.
*/
func (market *learningMarket) observe(mid, cost float64, at time.Time) {
	if !(mid > 0) {
		return
	}
	market.cost = cost

	if market.movement == nil {
		market.movement = adaptive.NewBaseline(adaptive.NewWindow())
	}

	if market.lastMid > 0 && at.After(market.observationAt) {
		market.observations++
		market.observationMean += (at.Sub(market.observationAt).Seconds() - market.observationMean) /
			float64(market.observations)

		reading := market.movement.Observe(math.Log(mid / market.lastMid))
		market.hasSigma = reading.Prior.Count >= movementReadiness && reading.PriorVariance > 0

		if market.hasSigma {
			market.sigma = math.Sqrt(reading.PriorVariance)
		}
	}

	market.lastMid, market.observationAt = mid, at
}

/*
horizon is the forward window every decision in this market is measured over:
the time this instrument's own movement needs before it could plausibly cover
the round trip the decision paid.

Price displacement grows with the square root of elapsed observations, so a
market whose per-observation movement is sigma covers a cost c after roughly
(c/sigma)^2 observations. That is the point at which a typical move is the size
of the friction — before it, the only thing the outcome can report is the
spread and the fees, and every action that costs anything scores as a loss
while waiting scores as exactly zero. It is the same random-walk scaling the
hindsight reviewer already uses to decide when a move is large enough to be an
excursion, applied to the question of when an outcome is large enough to be
evidence.

Deriving it this way makes the window the instrument's own. A liquid, fast
market answers a decision in seconds; an illiquid one with a wide spread needs
far longer, and honestly reports that it does rather than being judged on a
window borrowed from a different asset.

It is unavailable until both the movement and the cost have actually been
measured. An unmeasured window resolves nothing rather than inventing one.
*/
func (market *learningMarket) horizon() time.Duration {
	if !market.hasSigma || market.sigma <= 0 || market.cost <= 0 || market.observationMean <= 0 {
		return 0
	}
	observations := math.Pow(market.cost/market.sigma, 2)
	window := time.Duration(observations * market.observationMean * float64(time.Second))

	/*
		The ceiling is an attribution choice, not a physical one: beyond it, too
		much unrelated tape has passed for an outcome to be evidence about the
		decision that opened it. An instrument that needs longer than this to
		cover its own friction is reporting that it cannot be traded profitably
		at this size, which is a finding rather than a value to be adjusted.
	*/
	if ceiling := system.Cfg.Learning.MaximumHorizon; window > ceiling {
		return ceiling
	}

	if window <= 0 {
		return 0
	}

	return window
}

/*
AdvanceImpulse updates the market's observation state and advances the temporal
precursor history only when the ordered active regions actually change.

The history is bounded. Conditioning on an unbounded past does not deepen the
model: every context that carries the whole run is unique, so it trains a fresh
path that is never visited again, while the cost of binding and recalling it
grows with uptime. The retained depth is a declared operating choice.
*/
func (market *learningMarket) AdvanceImpulse(regions []learning.Region) bool {
	conditions := make([]uint64, len(regions))

	for index, region := range regions {
		conditions[index] = region.Condition
	}

	if slices.Equal(conditions, market.currentConditions) {
		return false
	}

	if len(market.currentConditions) > 0 {
		market.history = append(market.history, append([]uint64(nil), market.currentConditions...))
	}

	if frames := system.Cfg.Learning.PrecursorFrames; len(market.history) > frames {
		market.history = append(market.history[:0], market.history[len(market.history)-frames:]...)
	}

	market.currentConditions = conditions
	market.regions = append(market.regions[:0], regions...)
	market.exploration++

	market.authority = 0
	strength := 0.0

	for _, region := range regions {
		market.authority += region.Strength * region.Authority
		strength += region.Strength
	}

	if strength > 0 {
		market.authority /= strength
	}

	return true
}

/*
PrecursorContext arranges precursor history so that shorter prefixes correspond
to shorter recent precursor histories: current impulse first (in strength order),
followed by FRAME and previous impulses in reverse chronological order.
*/
func (market *learningMarket) PrecursorContext() []uint64 {
	if len(market.currentConditions) == 0 {
		return nil
	}

	size := len(market.currentConditions)

	for _, past := range market.history {
		size += 1 + len(past)
	}

	context := make([]uint64, 0, size)
	context = append(context, market.currentConditions...)

	for index := len(market.history) - 1; index >= 0; index-- {
		context = append(context, FrameDelimiter)
		context = append(context, market.history[index]...)
	}

	return context
}
