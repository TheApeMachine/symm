package types

import (
	"context"
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
StopEvidence is the thin numeric projection Stoploss consumes from one Thesis
cut (or a mark-only tick). Missing mark or entry leaves Present false so the
regulator freezes instead of inventing prices.
*/
type StopEvidence struct {
	Symbol               string
	Mark                 float64
	Entry                float64
	ForecastEpoch        uint64
	NormalizedResidual   float64
	ExpectedReturn       float64
	Uncertainty          float64
	IncrementalMSE       float64
	ReturnReady          bool
	CausalReady          bool
	CausalExpectedReturn float64
	CognitionReady       bool
	CognitionConfidence  float64
	CognitionWinner      string
	CognitionAmbiguous   bool
	Spread               float64
	SellCapacity         float64
	Present              bool
}

/*
Trail owns ratchet, arm-scale, and take-profit geometry for one Stoploss.
It keeps floor/trail math off the regulator shell so JSON fields stay flat.
*/
type Trail struct{}

/*
Skill tracks forecast-epoch skill reweighting for trail distance scaling.
*/
type Skill struct {
	lastConsumedEpoch uint64
}

/*
Stoploss is a numerical exit regulator for one open position. It keeps a
max-monotone lockedFloor, a separate trailDistance scaled by forecast
uncertainty and skill weight, and fires take-profit when peak proximity meets
a non-positive forward path or residual blowout — without named regimes.
Public Action/Reason fields are the live verdict surface for journals and UI.
*/
type Stoploss struct {
	ctx    context.Context
	cancel context.CancelFunc
	trail  Trail
	skill  Skill
	armed  bool
	entry  float64

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
		ctx:    ctx,
		cancel: cancel,
		Action: "hold",
		Reason: "unarmed",
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
Frame projects the live stop surface for the terminal gauge. stop_price is the
absolute floor implied by entry×(1+stopReturn) once armed; unarmed regulators
omit a positive floor so the UI hides the stop marker.
*/
func (stoploss *Stoploss) Frame(symbol string) map[string]any {
	if stoploss == nil || symbol == "" {
		return nil
	}

	frame := map[string]any{
		"symbol":      symbol,
		"peak_return": stoploss.PeakReturn,
		"stop_return": stoploss.StopReturn,
		"armed":       stoploss.armed,
		"stop_price":  0.0,
	}

	if stoploss.armed && stoploss.entry > 0 && !math.IsInf(stoploss.StopReturn, 0) {
		frame["stop_price"] = stoploss.entry * (1 + stoploss.StopReturn)
	}

	return frame
}

/*
Update consumes one Evidence cut and refreshes the public stop surface. Absent
Evidence freezes prior floors and weight so nil frames cannot ratchet or
unwind the stop through missing data.
*/
func (stoploss *Stoploss) Update(evidence StopEvidence) *Stoploss {
	if !evidence.Present {
		return stoploss.snapshot("hold", "evidence absent; frozen", 0)
	}

	armScale := stoploss.trail.armScale(evidence)

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
	stoploss.skill.reweight(stoploss, evidence)
	stoploss.TrailDistance = stoploss.trail.trailScale(evidence, armScale) / stoploss.Weight
	stoploss.trail.ratchet(stoploss, markReturn)

	if markReturn <= stoploss.StopReturn {
		return stoploss.snapshot(
			"stop", "mark returned through live stop", markReturn,
		)
	}

	if stoploss.trail.takeProfit(stoploss, evidence, markReturn) {
		return stoploss.snapshot(
			"take_profit",
			"peak proximity with non-positive forward path",
			markReturn,
		)
	}

	return stoploss.snapshot("hold", "stop armed; path intact", markReturn)
}

/*
Regulate applies one StopEvidence cut and appends an exit Decision when stop
or take_profit fires. Trade remains the sole order submission path.
*/
func (stoploss *Stoploss) Regulate(
	thesis *Thesis,
	holding Holding,
	evidence StopEvidence,
) {
	if stoploss == nil || thesis == nil || holding.Symbol == "" {
		return
	}

	if phase, found := thesis.Lifecycle.Load(holding.Symbol); found {
		if phase == LifecycleExitSelected ||
			phase == LifecycleExitSubmitted {
			return
		}
	}

	verdict := stoploss.Update(evidence)
	stoploss.Action = verdict.Action
	stoploss.Reason = verdict.Reason
	stoploss.MarkReturn = verdict.MarkReturn

	if verdict.Action != "stop" && verdict.Action != "take_profit" {
		return
	}

	thesis.Decisions = append(thesis.Decisions, Decision{
		Action:           "exit",
		Symbol:           holding.Symbol,
		At:               time.Now().UTC(),
		Utility:          stoploss.StopReturn,
		Alternatives:     map[string]float64{verdict.Action: stoploss.StopReturn},
		ProposedQuantity: holding.Qty,
		ReferencePrice:   decimal.NewFromFloat64(evidence.Mark),
		Cause:            verdict.Action,
		Reason:           verdict.Reason,
	})

	thesis.Lifecycle.Store(holding.Symbol, LifecycleExitSelected)
}

/*
ObserveMark ratchets an already-armed stop from a live mark print so Holding
tick updates move the floor without waiting for the next Thesis cut.
*/
func (stoploss *Stoploss) ObserveMark(mark float64) {
	if stoploss == nil || !stoploss.armed || stoploss.entry <= 0 || mark <= 0 {
		return
	}

	markReturn := (mark - stoploss.entry) / stoploss.entry
	stoploss.PeakReturn = math.Max(stoploss.PeakReturn, markReturn)
	stoploss.trail.ratchet(stoploss, markReturn)
	stoploss.MarkReturn = markReturn

	if markReturn <= stoploss.StopReturn {
		stoploss.Action = "stop"
		stoploss.Reason = "mark breached live stop on tick"
		return
	}

	stoploss.Action = "hold"
	stoploss.Reason = "stop armed; path intact"
}

/*
arm latches entry and seeds weight on the first Present Evidence cut that
carries a defensible arm scale.
*/
func (stoploss *Stoploss) arm(evidence StopEvidence) {
	stoploss.armed = true
	stoploss.entry = evidence.Entry
	stoploss.Weight = stoploss.skill.seed(evidence)
	stoploss.LockedFloor = math.Inf(-1)
	stoploss.PeakReturn = 0
}

/*
snapshot returns an independent copy of the live stop surface so callers can
read a frozen verdict without later Updates mutating prior results.
*/
func (stoploss *Stoploss) snapshot(
	action, reason string,
	markReturn float64,
) *Stoploss {
	copy := *stoploss
	copy.Action = action
	copy.Reason = reason
	copy.MarkReturn = markReturn

	return &copy
}

/*
ratchet advances LockedFloor only after peak has earned a positive cushion
above trail distance, then sets the live stop. Until that cushion exists the
stop is the adverse entry band (−trail), so early chop cannot invent a floor.
*/
func (trail *Trail) ratchet(stoploss *Stoploss, markReturn float64) {
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
func (skill *Skill) seed(evidence StopEvidence) float64 {
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
func (trail *Trail) armScale(evidence StopEvidence) float64 {
	if evidence.Uncertainty > 0 {
		return evidence.Uncertainty
	}

	if evidence.ReturnReady && evidence.IncrementalMSE > 0 {
		return math.Sqrt(evidence.IncrementalMSE)
	}

	return 0
}

/*
trailScale widens the armed trail by live spread when that exceeds arm σ, so
execution friction loosens the band without being allowed to arm it.
*/
func (trail *Trail) trailScale(evidence StopEvidence, armScale float64) float64 {
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
func (skill *Skill) reweight(stoploss *Stoploss, evidence StopEvidence) {
	if !evidence.ReturnReady {
		return
	}

	if evidence.ForecastEpoch == skill.lastConsumedEpoch {
		return
	}

	skill.lastConsumedEpoch = evidence.ForecastEpoch

	if evidence.NormalizedResidual < 0 {
		return
	}

	residualSkill := 1.0

	if evidence.NormalizedResidual > 0 {
		residualSkill = 1 / (1 + evidence.NormalizedResidual)
	}

	delta := residualSkill - stoploss.Weight

	if delta >= 0 {
		stoploss.Weight += delta * residualSkill * residualSkill
	}

	if delta < 0 {
		stoploss.Weight += delta * (1 - residualSkill)
	}

	stoploss.Weight = math.Min(1, math.Max(math.Nextafter(0, 1), stoploss.Weight))
}

/*
takeProfit reports whether mark sits near the peak while the forward path is
non-positive, residual-blown, or causally opposed.
*/
func (trail *Trail) takeProfit(
	stoploss *Stoploss,
	evidence StopEvidence,
	markReturn float64,
) bool {
	if stoploss.PeakReturn <= 0 {
		return false
	}

	proximity := stoploss.PeakReturn - markReturn
	nearPeak := proximity <= trail.trailScale(evidence, trail.armScale(evidence))

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
