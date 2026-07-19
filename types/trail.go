package types

import "math"

/*
Trail owns ratchet and take-profit geometry for one open lot. Stoploss embeds
it so JSON and callers see LockedFloor/StopReturn without a second surface.
*/
type Trail struct {
	LockedFloor   float64 `json:"lockedFloor"`
	FloorDistance float64 `json:"floorDistance"`
	TrailDistance float64 `json:"trailDistance"`
	StopReturn    float64 `json:"stopReturn"`
	PeakReturn    float64 `json:"peakReturn"`
	MarkReturn    float64 `json:"markReturn"`
}

/*
NewTrail returns an unbound trail with LockedFloor at −Inf.
*/
func NewTrail() Trail {
	return Trail{LockedFloor: math.Inf(-1)}
}

/*
Bind resets geometry at fill with an adverse entry band. FloorDistance latches
that bind width so later Scale cannot undercut fee/spread survival.
*/
func (trail *Trail) Bind(distance float64) {
	trail.LockedFloor = math.Inf(-1)
	trail.PeakReturn = 0
	trail.MarkReturn = 0
	trail.FloorDistance = 0

	if distance > 0 && !math.IsNaN(distance) && !math.IsInf(distance, 0) {
		trail.FloorDistance = distance
		trail.TrailDistance = distance
		trail.StopReturn = -distance
	}
}

/*
Advance records markReturn, ratchets the floor after an earned cushion, and
sets the live stop. Until that cushion exists the stop is −trail. Once a floor
exists StopReturn never loosens — pullbacks and Scale shrinks cannot give back
drawdown room the ratchet already claimed.
*/
func (trail *Trail) Advance(markReturn float64) {
	trail.MarkReturn = markReturn
	trail.PeakReturn = math.Max(trail.PeakReturn, markReturn)
	candidate := trail.PeakReturn - trail.TrailDistance

	if candidate > 0 {
		trail.LockedFloor = math.Max(trail.LockedFloor, candidate)
	}

	next := -trail.TrailDistance

	if !math.IsInf(trail.LockedFloor, -1) {
		next = math.Max(trail.LockedFloor, markReturn-trail.TrailDistance)
		next = math.Max(trail.StopReturn, next)
	}

	trail.StopReturn = next
}

/*
Scale sets TrailDistance from live evidence width divided by skill weight, never
below FloorDistance so high skill cannot shrink under the fill-time survival band.
Non-finite scale or weight leaves geometry untouched so Breached cannot go blind.
*/
func (trail *Trail) Scale(evidence StopEvidence, weight float64) {
	scale := trail.LiveScale(evidence)

	if scale <= 0 || weight <= 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
		return
	}

	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return
	}

	if evidence.Spread > scale {
		scale = evidence.Spread
	}

	next := scale / weight

	if math.IsNaN(next) || math.IsInf(next, 0) || next <= 0 {
		return
	}

	if trail.FloorDistance > 0 && next < trail.FloorDistance {
		next = trail.FloorDistance
	}

	trail.TrailDistance = next
}

/*
Breached reports whether markReturn has crossed the live stop.
*/
func (trail *Trail) Breached(markReturn float64) bool {
	return trail.TrailDistance > 0 && markReturn <= trail.StopReturn
}

/*
TakeProfit reports whether mark sits near the peak while the forward path is
non-positive, residual-blown, or causally opposed.
*/
func (trail *Trail) TakeProfit(evidence StopEvidence, markReturn float64) bool {
	if trail.PeakReturn <= 0 {
		return false
	}

	band := trail.LiveScale(evidence)

	if evidence.Spread > band {
		band = evidence.Spread
	}

	if trail.PeakReturn-markReturn > band {
		return false
	}

	if evidence.ReturnReady && evidence.ExpectedReturn <= 0 {
		return true
	}

	if evidence.NormalizedResidual > 1 {
		return true
	}

	return evidence.CausalReady && evidence.CausalExpectedReturn < 0
}

/*
LiveScale is the return-space width available now: forecast σ, residual RMSE,
or visible spread.
*/
func (trail *Trail) LiveScale(evidence StopEvidence) float64 {
	scale := 0.0

	if evidence.Uncertainty > 0 {
		scale = evidence.Uncertainty
	}

	if evidence.ReturnReady && evidence.IncrementalMSE > 0 {
		scale = math.Max(scale, math.Sqrt(evidence.IncrementalMSE))
	}

	if evidence.Spread > 0 {
		scale = math.Max(scale, evidence.Spread)
	}

	return scale
}
