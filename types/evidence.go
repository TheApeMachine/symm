package types

import (
	"time"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
)

/*
evidenceFreshness is how long a strategy reading may keep gating stop geometry.

It is generous relative to the tick rate and deliberately finite: a stalled
analysis pipeline that stops publishing must not leave a stale toxicity reading
suppressing every peak for the rest of the position's life.
*/
const evidenceFreshness = 30 * time.Second

/*
StopEvidence is one observation of an open lot: the price a sale of it would
actually realise right now, and whether that price is trustworthy enough to
move the stop's geometry.

It carries only what the regulator consumes. Forecast return, causal uplift and
cognition confidence used to be declared here and were never read. Structural
regime invalidation and live execution cost remain because they answer a
different question: whether the market in which the frozen loss geometry was
admitted still exists. Duplicating ordinary continuation scores into the stop
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
		DepthLimited is true when the visible book could not absorb the whole
		position, so the mark above is a real quoted price that is only true of
		the part that fits. Consumers must not build profit geometry on it.
	*/
	DepthLimited bool `json:"depth_limited"`
	/*
		ObservedAt is when the strategy took this reading. Without it a latched
		observation is indistinguishable from a current one, and a stalled
		analysis pipeline would go on suppressing peaks or widening trails from
		a book that has since moved. Consumers age it out rather than trusting
		whatever was last stored.
	*/
	ObservedAt time.Time `json:"observed_at"`
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
		HollowPressure is the share of the bid touch that is being pulled rather
		than filled: cancelled quantity over what was resting there before it
		went. A rising bid backed by size that keeps disappearing is a price
		nothing could have been sold into, and ratcheting a floor up to meet it
		would exit on liquidity that was never there.

		This reads the cancellation metric, not the retreat one. Toxicity emits
		them for different events: a buy-side "retreat" is the bid stepping
		*down*, which produces no new peak to suppress in the first place, while
		a cancellation is size vanishing at an unchanged best price — exactly
		the hollow quote this is meant to catch. The earlier version gated on
		retreat and therefore never fired when it mattered.

		It suppresses geometry, not exits. A hollow bid is evidence of execution
		danger, not of safety, and the loss boundary goes on judging the mark.
	*/
	HollowPressure float64 `json:"hollow_pressure"`
	/*
		HollowReady is true when this cut observed a toxicity measurement, so
		the regulator can tell "nothing being pulled" from "no reading".
	*/
	HollowReady bool `json:"hollow_ready"`
	Present     bool `json:"present"`
}

/*
hollowMateriality is how much of the touch must be pulled before a quote is
treated as untrustworthy.

Some cancellation is ordinary market making, and gating on any positive value at
all suppressed peaks continuously in a normally functioning book — which quietly
pinned the trail at entry for the whole life of a position. A fifth of the touch
disappearing is a different claim from a few percent.
*/
const hollowMateriality = 0.2

/*
GeometryValid reports whether this observation may set a new peak.

Absent evidence is treated as valid: a mark that arrived without a toxicity
reading is the ordinary case, and freezing the peak whenever the strategy has
not spoken would leave the trail permanently anchored at entry. Stale evidence
is treated the same way — a reading nobody has refreshed in half a minute is not
a reading about this book.
*/
func (evidence StopEvidence) GeometryValid() bool {
	return evidence.GeometryValidAt(time.Now().UTC())
}

/*
Fresh reports whether the matched strategy reading may still update execution
noise. A missing or future timestamp is not a current observation.
*/
func (evidence StopEvidence) Fresh(now time.Time) bool {
	if evidence.ObservedAt.IsZero() {
		return false
	}

	age := now.Sub(evidence.ObservedAt)
	return age >= 0 && age <= evidenceFreshness
}

/*
GeometryValidAt reports whether this observation may set a new peak at now.
*/
func (evidence StopEvidence) GeometryValidAt(now time.Time) bool {
	if !evidence.HollowReady || evidence.HollowPressure <= hollowMateriality {
		return true
	}

	// A material reading with no time on it cannot be aged out, so it is
	// honoured rather than assumed current.
	if evidence.ObservedAt.IsZero() {
		return false
	}

	age := now.Sub(evidence.ObservedAt)
	return age > evidenceFreshness
}
