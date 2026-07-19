package types

import (
	"context"
	"math"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Stoploss is a numerical exit regulator for one open position. It composes
Trail geometry and Skill weight, and fires take-profit when peak proximity
meets a non-positive forward path or residual blowout — without named regimes.
Action/Reason are the live verdict surface for journals and UI.
*/
type Stoploss struct {
	ctx     context.Context
	cancel  context.CancelFunc
	armed   bool
	entry   float64
	retreat float64

	Trail
	Skill

	Action string `json:"action"`
	Reason string `json:"reason"`
}

/*
NewStoploss constructs a regulator shell. Bind latches entry at fill so an open
lot is never unprotected waiting for forecast σ.
*/
func NewStoploss(ctx context.Context) *Stoploss {
	ctx, cancel := context.WithCancel(ctx)

	return &Stoploss{
		ctx:    ctx,
		cancel: cancel,
		Trail:  NewTrail(),
		Skill:  NewSkill(),
		Action: "hold",
		Reason: "unbound",
	}
}

/*
Bind latches entry and an initial adverse trail at fill. Trail may be fee or
spread width in return space; later Update only rescales, never waits to exist.
A non-positive or non-finite distance refuses to arm so Breached cannot go blind.
*/
func (stoploss *Stoploss) Bind(entry, distance float64) {
	if stoploss == nil || entry <= 0 || (stoploss.armed && stoploss.entry > 0) {
		return
	}

	if distance <= 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
		stoploss.Action = "hold"
		stoploss.Reason = "bind refused; trail width absent"
		return
	}

	stoploss.armed = true
	stoploss.entry = entry
	stoploss.Action = "hold"
	stoploss.Reason = "bound at entry"
	stoploss.Trail.Bind(distance)
}

/*
Restore arms a regulator from durable recovery state without resetting Trail
geometry. Bind is for fresh fills only — recovery must keep LockedFloor and
peak/mark returns.
*/
func (stoploss *Stoploss) Restore(entry float64, recovered *Stoploss) {
	if stoploss == nil || recovered == nil || entry <= 0 {
		return
	}

	stoploss.armed = true
	stoploss.entry = entry
	stoploss.retreat = recovered.retreat
	stoploss.Trail = recovered.Trail
	stoploss.Skill = recovered.Skill
	stoploss.Weight = finiteFloat(stoploss.Weight)

	if !math.IsInf(stoploss.LockedFloor, -1) {
		stoploss.LockedFloor = finiteFloat(stoploss.LockedFloor)
	}

	stoploss.TrailDistance = finiteFloat(stoploss.TrailDistance)
	stoploss.FloorDistance = finiteFloat(stoploss.FloorDistance)
	stoploss.StopReturn = finiteFloat(stoploss.StopReturn)
	stoploss.PeakReturn = finiteFloat(stoploss.PeakReturn)
	stoploss.MarkReturn = finiteFloat(stoploss.MarkReturn)
	stoploss.Action = recovered.Action
	stoploss.Reason = recovered.Reason

	if stoploss.Action == "" {
		stoploss.Action = "hold"
	}

	if stoploss.Reason == "" {
		stoploss.Reason = "restored"
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
WidenSurvival raises the unlocked fill-time survival band when live fee and
half-spread evidence exceeds the cold-bind floor. Once LockedFloor is earned the
ratchet owns geometry and this is a no-op so sincere drawdown exits stay tight.
*/
func (stoploss *Stoploss) WidenSurvival(distance float64) {
	if stoploss == nil || !stoploss.armed {
		return
	}

	if distance <= 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
		return
	}

	if !math.IsInf(stoploss.LockedFloor, -1) {
		return
	}

	if distance <= stoploss.FloorDistance {
		return
	}

	stoploss.FloorDistance = distance

	if stoploss.TrailDistance < distance {
		stoploss.TrailDistance = distance
	}

	stoploss.StopReturn = -stoploss.TrailDistance
	stoploss.Reason = "survival band widened from live entry trail"
}

/*
healWidth restores a positive finite TrailDistance from live evidence when an
armed lot lost its survival band (NaN scale or zero-width recovery bind).
*/
func (stoploss *Stoploss) healWidth(evidence StopEvidence) {
	if stoploss == nil || !stoploss.armed {
		return
	}

	if stoploss.TrailDistance > 0 &&
		!math.IsNaN(stoploss.TrailDistance) &&
		!math.IsInf(stoploss.TrailDistance, 0) {
		return
	}

	distance := stoploss.Trail.LiveScale(evidence)

	if distance <= 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
		return
	}

	if stoploss.FloorDistance <= 0 {
		stoploss.FloorDistance = distance
	}

	stoploss.TrailDistance = distance

	if stoploss.StopReturn == 0 || math.IsNaN(stoploss.StopReturn) ||
		math.IsInf(stoploss.StopReturn, 0) {
		stoploss.StopReturn = -distance
	}

	stoploss.Reason = "trail width healed from live evidence"
}

/*
Armed reports whether the regulator has latched an entry and live floor.
*/
func (stoploss *Stoploss) Armed() bool {
	return stoploss != nil && stoploss.armed
}

/*
StopPrice returns the absolute floor implied by entry×(1+stopReturn).
Non-finite geometry returns 0 so the UI hides a broken marker.
*/
func (stoploss *Stoploss) StopPrice() float64 {
	if stoploss == nil || !stoploss.armed || stoploss.entry <= 0 {
		return 0
	}

	if math.IsNaN(stoploss.StopReturn) || math.IsInf(stoploss.StopReturn, 0) {
		return 0
	}

	price := stoploss.entry * (1 + stoploss.StopReturn)

	if math.IsNaN(price) || math.IsInf(price, 0) {
		return 0
	}

	return price
}

/*
Update consumes one Evidence cut and refreshes the public stop surface.
*/
func (stoploss *Stoploss) Update(evidence StopEvidence) *Stoploss {
	if !evidence.Present {
		return stoploss.settle("hold", "evidence absent; frozen", stoploss.MarkReturn)
	}

	if evidence.Entry <= 0 || evidence.Mark <= 0 {
		return stoploss.settle("hold", "prices absent; frozen", stoploss.MarkReturn)
	}

	if !stoploss.armed {
		distance := stoploss.TrailDistance

		if distance <= 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
			distance = stoploss.Trail.LiveScale(evidence)
		}

		stoploss.Bind(evidence.Entry, distance)
		stoploss.Skill.Seed(evidence)
	}

	if stoploss.armed {
		stoploss.healWidth(evidence)
	}

	if evidence.RetreatReady {
		stoploss.retreat = math.Max(0, evidence.RetreatPressure)
	}

	markReturn := (evidence.Mark - stoploss.entry) / stoploss.entry

	if stoploss.retreat > 0 {
		return stoploss.settle("hold", "retreat-driven mark; geometry frozen", markReturn)
	}

	if !stoploss.armed {
		return stoploss.settle("hold", "stop unbound; trail width absent", markReturn)
	}

	stoploss.Skill.Reweight(evidence)
	stoploss.Trail.Scale(evidence, stoploss.Weight)
	stoploss.Trail.Advance(markReturn)

	if stoploss.Trail.Breached(markReturn) {
		return stoploss.settle("stop", "mark returned through live stop", markReturn)
	}

	if stoploss.Trail.TakeProfit(evidence, markReturn) {
		return stoploss.settle(
			"take_profit",
			"peak proximity with non-positive forward path",
			markReturn,
		)
	}

	return stoploss.settle("hold", "stop live; path intact", markReturn)
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
		if phase == LifecycleExitSelected || phase == LifecycleExitSubmitted {
			return
		}
	}

	stoploss.Update(evidence)

	if stoploss.Action != "stop" && stoploss.Action != "take_profit" {
		return
	}

	thesis.Decisions = append(thesis.Decisions, Decision{
		Action:           "exit",
		Symbol:           holding.Symbol,
		At:               thesis.At,
		Utility:          stoploss.StopReturn,
		Alternatives:     map[string]float64{stoploss.Action: stoploss.StopReturn},
		ProposedQuantity: holding.Qty,
		ReferencePrice:   decimal.NewFromFloat64(evidence.Mark),
		Cause:            stoploss.Action,
		Reason:           stoploss.Reason,
	})
	thesis.NoteLifecycle(holding.Symbol, LifecycleExitSelected, thesis.At)
}

/*
ObserveMark ratchets a bound stop from a live mark print. It never sets Action
to stop or take_profit — only Regulate emits exit Decisions.
*/
func (stoploss *Stoploss) ObserveMark(mark float64) {
	if stoploss == nil || !stoploss.armed || stoploss.entry <= 0 || mark <= 0 {
		return
	}

	markReturn := (mark - stoploss.entry) / stoploss.entry
	stoploss.MarkReturn = markReturn
	stoploss.Action = "hold"

	if stoploss.retreat > 0 {
		stoploss.Reason = "retreat-gated mark; await sincere print"
		return
	}

	stoploss.Trail.Advance(markReturn)
	stoploss.Reason = "stop live; path intact"

	if stoploss.Trail.Breached(markReturn) {
		stoploss.Reason = "mark breached live stop; await regulate"
	}
}

/*
NoteRetreat latches toxicity retreat pressure between Thesis cuts so tick
marks from Marks.Apply share the same sincerity gate as Update.
*/
func (stoploss *Stoploss) NoteRetreat(pressure float64) {
	if stoploss != nil {
		stoploss.retreat = math.Max(0, pressure)
	}
}

/*
MarshalJSON encodes a JSON-safe stop surface. FloorLocked preserves the
meaning of the non-finite unlocked sentinel while scalar geometry remains JSON
finite. Retreat is persisted so recovery restarts keep the sincerity gate.
*/
func (stoploss Stoploss) MarshalJSON() ([]byte, error) {
	type wire struct {
		Action        string  `json:"action"`
		Reason        string  `json:"reason"`
		Weight        float64 `json:"weight"`
		LockedFloor   float64 `json:"lockedFloor"`
		FloorLocked   bool    `json:"floorLocked"`
		FloorDistance float64 `json:"floorDistance"`
		TrailDistance float64 `json:"trailDistance"`
		StopReturn    float64 `json:"stopReturn"`
		PeakReturn    float64 `json:"peakReturn"`
		MarkReturn    float64 `json:"markReturn"`
		Retreat       float64 `json:"retreat"`
	}

	return sonic.Marshal(wire{
		Action:        stoploss.Action,
		Reason:        stoploss.Reason,
		Weight:        finiteFloat(stoploss.Weight),
		LockedFloor:   finiteFloat(stoploss.LockedFloor),
		FloorLocked:   !math.IsInf(stoploss.LockedFloor, -1),
		FloorDistance: finiteFloat(stoploss.FloorDistance),
		TrailDistance: finiteFloat(stoploss.TrailDistance),
		StopReturn:    finiteFloat(stoploss.StopReturn),
		PeakReturn:    finiteFloat(stoploss.PeakReturn),
		MarkReturn:    finiteFloat(stoploss.MarkReturn),
		Retreat:       finiteFloat(stoploss.retreat),
	})
}

/*
UnmarshalJSON restores the stop surface, unlocked-floor state, and private
retreat gate without inventing a break-even ratchet or clearing sincerity.
*/
func (stoploss *Stoploss) UnmarshalJSON(payload []byte) error {
	if stoploss == nil {
		return errnie.Err(
			errnie.Validation,
			"stoploss: UnmarshalJSON on nil receiver",
			nil,
		)
	}

	type wire struct {
		Action        string  `json:"action"`
		Reason        string  `json:"reason"`
		Weight        float64 `json:"weight"`
		LockedFloor   float64 `json:"lockedFloor"`
		FloorLocked   bool    `json:"floorLocked"`
		FloorDistance float64 `json:"floorDistance"`
		TrailDistance float64 `json:"trailDistance"`
		StopReturn    float64 `json:"stopReturn"`
		PeakReturn    float64 `json:"peakReturn"`
		MarkReturn    float64 `json:"markReturn"`
		Retreat       float64 `json:"retreat"`
	}

	var frame wire

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return err
	}

	stoploss.Action = frame.Action
	stoploss.Reason = frame.Reason
	stoploss.Weight = finiteFloat(frame.Weight)
	stoploss.LockedFloor = finiteFloat(frame.LockedFloor)

	if !frame.FloorLocked && frame.LockedFloor <= 0 {
		stoploss.LockedFloor = math.Inf(-1)
	}

	stoploss.FloorDistance = finiteFloat(frame.FloorDistance)
	stoploss.TrailDistance = finiteFloat(frame.TrailDistance)
	stoploss.StopReturn = finiteFloat(frame.StopReturn)
	stoploss.PeakReturn = finiteFloat(frame.PeakReturn)
	stoploss.MarkReturn = finiteFloat(frame.MarkReturn)
	stoploss.retreat = finiteFloat(frame.Retreat)

	return nil
}

func (stoploss *Stoploss) settle(action, reason string, markReturn float64) *Stoploss {
	stoploss.Action = action
	stoploss.Reason = reason
	stoploss.MarkReturn = markReturn

	return stoploss
}
