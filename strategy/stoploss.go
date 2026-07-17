package strategy

import (
	"context"
	"math"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
Stoploss is a numerical exit regulator for one open position. It keeps a
max-monotone lockedFloor, a separate trailDistance scaled by forecast
uncertainty and skill weight, and fires take-profit when peak proximity meets
a non-positive forward path or residual blowout — without named regimes.
Public Action/Reason fields are the live verdict surface for journals and UI.
*/
type Stoploss struct {
	ctx               context.Context
	cancel            context.CancelFunc
	armed             bool
	entry             float64
	lastConsumedEpoch uint64

	Action        string  `json:"action"`
	Reason        string  `json:"reason"`
	Weight        float64 `json:"weight"`
	LockedFloor   float64 `json:"lockedFloor"`
	TrailDistance float64 `json:"trailDistance"`
	StopReturn    float64 `json:"stopReturn"`
	PeakReturn    float64 `json:"peakReturn"`
	MarkReturn    float64 `json:"markReturn"`
}

/*
NewStoploss constructs an unarmed regulator. Weight starts unset until the
first Present Evidence supplies cognition or forecast confidence.
*/
func NewStoploss(ctx context.Context) *Stoploss {
	ctx, cancel := context.WithCancel(ctx)

	return &Stoploss{
		ctx:               ctx,
		cancel:            cancel,
		lastConsumedEpoch: ^uint64(0),
		Action:            "hold",
		Reason:            "unarmed",
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
Update consumes one Evidence cut and refreshes the public stop surface. Absent
Evidence freezes prior floors and weight so nil frames cannot ratchet or
unwind the stop through missing data.
*/
func (stoploss *Stoploss) Update(evidence Evidence) *Stoploss {
	if !evidence.Present {
		return stoploss.snapshot("hold", "evidence absent; frozen", 0)
	}

	armScale := stoploss.armScale(evidence)

	if armScale <= 0 {
		if !stoploss.armed {
			return stoploss.snapshot("hold", "waiting for trail scale", 0)
		}

		markReturn := (evidence.Mark - stoploss.entry) / stoploss.entry

		return stoploss.snapshot("hold", "trail scale absent; frozen", markReturn)
	}

	if !stoploss.armed {
		stoploss.arm(evidence)
	}

	markReturn := (evidence.Mark - stoploss.entry) / stoploss.entry
	stoploss.PeakReturn = math.Max(stoploss.PeakReturn, markReturn)
	stoploss.reweight(evidence)
	stoploss.TrailDistance = stoploss.trailScale(evidence, armScale) / stoploss.Weight
	stoploss.ratchet(markReturn)

	if markReturn <= stoploss.StopReturn {
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
Regulate projects Thesis evidence for one holding and appends an exit Decision
when stop or take_profit fires. Trade remains the sole order submission path.
*/
func (stoploss *Stoploss) Regulate(thesis *types.Thesis, holding types.Holding) {
	if thesis == nil || holding.Symbol == "" {
		return
	}

	evidence := Project(thesis, holding)
	stoploss.Update(evidence)

	if stoploss.Action != "stop" && stoploss.Action != "take_profit" {
		return
	}

	quantity := 0.0

	if holding.Qty != nil {
		quantity = holding.Qty.Float64()
	}

	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action:           "exit",
		Symbol:           holding.Symbol,
		At:               time.Now().UTC(),
		Utility:          stoploss.StopReturn,
		Alternatives:     map[string]float64{stoploss.Action: stoploss.StopReturn},
		ProposedQuantity: quantity,
		ReferencePrice:   evidence.Mark,
		Cause:            stoploss.Action,
		Reason:           stoploss.Reason,
	})

	thesis.Lifecycle.Store(holding.Symbol, types.LifecycleExitSelected)
}

/*
arm latches entry and seeds weight on the first Present Evidence cut that
carries a defensible arm scale.
*/
func (stoploss *Stoploss) arm(evidence Evidence) {
	stoploss.armed = true
	stoploss.entry = evidence.Entry
	stoploss.Weight = stoploss.seed(evidence)
	stoploss.LockedFloor = math.Inf(-1)
	stoploss.PeakReturn = 0
}

/*
ratchet advances LockedFloor only after peak has earned a positive cushion
above trail distance, then sets the live stop. Until that cushion exists the
stop is the adverse entry band (−trail), so early chop cannot invent a floor.
*/
func (stoploss *Stoploss) ratchet(markReturn float64) {
	candidate := stoploss.PeakReturn - stoploss.TrailDistance

	if candidate > 0 {
		stoploss.LockedFloor = math.Max(stoploss.LockedFloor, candidate)
	}

	stoploss.StopReturn = -stoploss.TrailDistance

	if !math.IsInf(stoploss.LockedFloor, -1) {
		stoploss.StopReturn = math.Max(
			stoploss.LockedFloor,
			markReturn-stoploss.TrailDistance,
		)
	}
}

/*
seed picks initial weight from the uncertainty share of expected-return
magnitude when σ is present, else cognition confidence. Preferring σ keeps a
high cold DMT posterior from collapsing trail distance on first arm.
*/
func (stoploss *Stoploss) seed(evidence Evidence) float64 {
	if evidence.Uncertainty > 0 {
		magnitude := math.Abs(evidence.ExpectedReturn) + evidence.Uncertainty

		return evidence.Uncertainty / magnitude
	}

	if evidence.CognitionReady &&
		evidence.CognitionConfidence > 0 &&
		evidence.CognitionConfidence <= 1 {
		return evidence.CognitionConfidence
	}

	return 1
}

/*
armScale is the return-space σ that may arm the regulator. Spread alone is
never enough — that path produced knife-edge trails on thin books.
*/
func (stoploss *Stoploss) armScale(evidence Evidence) float64 {
	if evidence.Uncertainty > 0 {
		return evidence.Uncertainty
	}

	if evidence.ReturnReady && evidence.IncrementalMSE > 0 {
		return evidence.IncrementalMSE
	}

	return 0
}

/*
trailScale widens the armed trail by live spread when that exceeds arm σ, so
execution friction loosens the band without being allowed to arm it.
*/
func (stoploss *Stoploss) trailScale(evidence Evidence, armScale float64) float64 {
	if evidence.Spread > armScale {
		return evidence.Spread
	}

	return armScale
}

/*
reweight moves skill weight toward residual-relative skill once per resolved
forecast epoch. Upside updates are damped so a few lucky residuals cannot
collapse trail distance pro-cyclically; downside updates cut faster when skill
is poor.
*/
func (stoploss *Stoploss) reweight(evidence Evidence) {
	if !evidence.ReturnReady {
		return
	}

	if evidence.ForecastEpoch == stoploss.lastConsumedEpoch {
		return
	}

	stoploss.lastConsumedEpoch = evidence.ForecastEpoch

	if evidence.NormalizedResidual <= 0 {
		return
	}

	skill := 1 / (1 + evidence.NormalizedResidual)
	delta := skill - stoploss.Weight

	if delta >= 0 {
		stoploss.Weight += delta * skill * skill
	}

	if delta < 0 {
		stoploss.Weight += delta * (1 - skill)
	}

	stoploss.Weight = math.Min(1, math.Max(math.Nextafter(0, 1), stoploss.Weight))
}

/*
takeProfit reports whether mark sits near the peak while the forward path is
non-positive, residual-blown, or causally opposed.
*/
func (stoploss *Stoploss) takeProfit(evidence Evidence, markReturn float64) bool {
	if stoploss.PeakReturn <= 0 {
		return false
	}

	proximity := stoploss.PeakReturn - markReturn
	nearPeak := proximity <= stoploss.trailScale(evidence, stoploss.armScale(evidence))

	if !nearPeak {
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
snapshot writes the live surface onto Stoploss itself and returns the receiver
so callers can chain field reads without a separate Verdict type.
*/
func (stoploss *Stoploss) snapshot(
	action, reason string,
	markReturn float64,
) *Stoploss {
	stoploss.Action = action
	stoploss.Reason = reason
	stoploss.MarkReturn = markReturn

	return stoploss
}
