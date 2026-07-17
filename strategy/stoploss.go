package strategy

import (
	"context"
	"math"
)

/*
Verdict is one Stoploss evaluation. Action is hold, stop, or take_profit; the
remaining fields expose the numbers that produced the choice for journals and
UI without re-deriving them from mark history.
*/
type Verdict struct {
	Action        string
	Reason        string
	Weight        float64
	LockedFloor   float64
	TrailDistance float64
	StopReturn    float64
	PeakReturn    float64
	MarkReturn    float64
}

/*
Stoploss is a numerical exit regulator for one open position. It keeps a
max-monotone lockedFloor, a separate trailDistance scaled by forecast
uncertainty and skill weight, and fires take-profit when peak proximity meets
a non-positive forward path or residual blowout — without named regimes.
*/
type Stoploss struct {
	ctx           context.Context
	cancel        context.CancelFunc
	armed         bool
	entry         float64
	peakReturn    float64
	lockedFloor   float64
	trailDistance float64
	weight        float64
	stopReturn    float64
}

/*
NewStoploss constructs an unarmed regulator. Weight starts unset until the
first Present Evidence supplies cognition or forecast confidence.
*/
func NewStoploss(ctx context.Context) *Stoploss {
	ctx, cancel := context.WithCancel(ctx)

	return &Stoploss{
		ctx:    ctx,
		cancel: cancel,
	}
}

/*
Close releases the regulator context during position teardown.
*/
func (stoploss *Stoploss) Close() {
	if stoploss.cancel != nil {
		stoploss.cancel()
	}
}

/*
Update consumes one Evidence cut and returns the live verdict. Absent Evidence
freezes prior floors and weight so nil frames cannot ratchet or unwind the
stop through missing data.
*/
func (stoploss *Stoploss) Update(evidence Evidence) Verdict {
	if !evidence.Present {
		return stoploss.snapshot("hold", "evidence absent; frozen", 0)
	}

	if !stoploss.armed {
		stoploss.arm(evidence)
	}

	markReturn := (evidence.Mark - stoploss.entry) / stoploss.entry
	stoploss.peakReturn = math.Max(stoploss.peakReturn, markReturn)
	stoploss.reweight(evidence)
	stoploss.trailDistance = stoploss.scale(evidence) / stoploss.weight
	candidate := stoploss.peakReturn - stoploss.trailDistance
	stoploss.lockedFloor = math.Max(stoploss.lockedFloor, candidate)
	stoploss.stopReturn = math.Max(
		stoploss.lockedFloor,
		markReturn-stoploss.trailDistance,
	)

	if markReturn <= stoploss.stopReturn {
		return stoploss.snapshot(
			"stop", "mark returned through live stop", markReturn,
		)
	}

	if stoploss.takeProfit(evidence, markReturn) {
		return stoploss.snapshot(
			"take_profit",
			"peak proximity with non-positive forward path",
			markReturn,
		)
	}

	return stoploss.snapshot("hold", "stop armed; path intact", markReturn)
}

/*
State exposes the current numerical stop surface for broker StopData / UI.
*/
func (stoploss *Stoploss) State() (
	armed bool,
	entry, peakReturn, stopReturn, weight, trail, floor float64,
) {
	return stoploss.armed, stoploss.entry, stoploss.peakReturn,
		stoploss.stopReturn, stoploss.weight, stoploss.trailDistance,
		stoploss.lockedFloor
}

/*
arm latches entry and seeds weight on the first Present Evidence cut.
*/
func (stoploss *Stoploss) arm(evidence Evidence) {
	stoploss.armed = true
	stoploss.entry = evidence.Entry
	stoploss.weight = stoploss.seed(evidence)
	stoploss.lockedFloor = math.Inf(-1)
	stoploss.peakReturn = 0
}

/*
seed picks initial weight from cognition confidence, else uncertainty share of
expected-return magnitude, so the first trail is data-derived.
*/
func (stoploss *Stoploss) seed(evidence Evidence) float64 {
	if evidence.CognitionReady &&
		evidence.CognitionConfidence > 0 &&
		evidence.CognitionConfidence <= 1 {
		return evidence.CognitionConfidence
	}

	if evidence.Uncertainty > 0 {
		magnitude := math.Abs(evidence.ExpectedReturn) + evidence.Uncertainty

		return evidence.Uncertainty / magnitude
	}

	return math.Nextafter(0, 1)
}

/*
scale returns the return-space uncertainty used for trail and peak proximity.
*/
func (stoploss *Stoploss) scale(evidence Evidence) float64 {
	if evidence.Uncertainty > 0 {
		return evidence.Uncertainty
	}

	if evidence.IncrementalMSE > 0 {
		return evidence.IncrementalMSE
	}

	if evidence.Spread > 0 {
		return evidence.Spread
	}

	return math.Nextafter(0, 1)
}

/*
reweight moves skill weight toward residual-relative skill. Upside updates are
damped so a few lucky residuals cannot collapse trail distance pro-cyclically;
downside updates cut faster when skill is poor.
*/
func (stoploss *Stoploss) reweight(evidence Evidence) {
	if !evidence.ReturnReady {
		return
	}

	scale := stoploss.scale(evidence)
	skill := scale / (scale + evidence.IncrementalMSE)
	delta := skill - stoploss.weight

	if delta >= 0 {
		stoploss.weight += delta * skill * skill
	}

	if delta < 0 {
		stoploss.weight += delta * (1 - skill)
	}

	stoploss.weight = math.Min(1, math.Max(math.Nextafter(0, 1), stoploss.weight))
}

/*
takeProfit reports whether mark sits near the peak while the forward path is
non-positive, residual-blown, or causally opposed.
*/
func (stoploss *Stoploss) takeProfit(evidence Evidence, markReturn float64) bool {
	if stoploss.peakReturn <= 0 {
		return false
	}

	proximity := stoploss.peakReturn - markReturn
	nearPeak := proximity <= stoploss.scale(evidence)

	if !nearPeak {
		return false
	}

	if evidence.ReturnReady && evidence.ExpectedReturn <= 0 {
		return true
	}

	if evidence.Uncertainty > 0 &&
		evidence.IncrementalMSE > evidence.Uncertainty {
		return true
	}

	return evidence.CausalReady && evidence.CausalExpectedReturn < 0
}

/*
snapshot copies the live surface into a Verdict for the caller.
*/
func (stoploss *Stoploss) snapshot(
	action, reason string,
	markReturn float64,
) Verdict {
	return Verdict{
		Action:        action,
		Reason:        reason,
		Weight:        stoploss.weight,
		LockedFloor:   stoploss.lockedFloor,
		TrailDistance: stoploss.trailDistance,
		StopReturn:    stoploss.stopReturn,
		PeakReturn:    stoploss.peakReturn,
		MarkReturn:    markReturn,
	}
}
