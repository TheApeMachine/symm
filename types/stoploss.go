package types

import (
	"context"
	"math"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
*/
func (stoploss *Stoploss) Bind(entry, distance float64) {
	if stoploss == nil || entry <= 0 || (stoploss.armed && stoploss.entry > 0) {
		return
	}

	stoploss.armed = true
	stoploss.entry = entry
	stoploss.Action = "hold"
	stoploss.Reason = "bound at entry"
	stoploss.Trail.Bind(distance)
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

		if distance <= 0 {
			distance = stoploss.Trail.LiveScale(evidence)
		}

		stoploss.Bind(evidence.Entry, distance)
		stoploss.Skill.Seed(evidence)
	}

	stoploss.retreat = math.Max(0, evidence.RetreatPressure)
	markReturn := (evidence.Mark - stoploss.entry) / stoploss.entry

	if stoploss.retreat > 0 {
		return stoploss.settle("hold", "retreat-driven mark; geometry frozen", markReturn)
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
		At:               time.Now().UTC(),
		Utility:          stoploss.StopReturn,
		Alternatives:     map[string]float64{stoploss.Action: stoploss.StopReturn},
		ProposedQuantity: holding.Qty,
		ReferencePrice:   decimal.NewFromFloat64(evidence.Mark),
		Cause:            stoploss.Action,
		Reason:           stoploss.Reason,
	})
	thesis.NoteLifecycle(holding.Symbol, LifecycleExitSelected, time.Now().UTC())
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
MarshalJSON encodes a JSON-safe stop surface. LockedFloor starts at −Inf after
arm; non-finite geometry is zeroed so Holding snapshots cannot break websocket
marshal.
*/
func (stoploss Stoploss) MarshalJSON() ([]byte, error) {
	type wire struct {
		Action        string  `json:"action"`
		Reason        string  `json:"reason"`
		Weight        float64 `json:"weight"`
		LockedFloor   float64 `json:"lockedFloor"`
		TrailDistance float64 `json:"trailDistance"`
		StopReturn    float64 `json:"stopReturn"`
		PeakReturn    float64 `json:"peakReturn"`
		MarkReturn    float64 `json:"markReturn"`
	}

	return sonic.Marshal(wire{
		Action:        stoploss.Action,
		Reason:        stoploss.Reason,
		Weight:        finiteFloat(stoploss.Weight),
		LockedFloor:   finiteFloat(stoploss.LockedFloor),
		TrailDistance: finiteFloat(stoploss.TrailDistance),
		StopReturn:    finiteFloat(stoploss.StopReturn),
		PeakReturn:    finiteFloat(stoploss.PeakReturn),
		MarkReturn:    finiteFloat(stoploss.MarkReturn),
	})
}

func (stoploss *Stoploss) settle(action, reason string, markReturn float64) *Stoploss {
	stoploss.Action = action
	stoploss.Reason = reason
	stoploss.MarkReturn = markReturn

	return stoploss
}
