package types

import (
	"context"
	"math"
	"sync"

	"github.com/theapemachine/errnie"
)

/*
Stoploss exits an open lot at a lower floor or an upper take. The floor
ratchets up under a peak; the band widens or narrows from live evidence.
LockedFloor is zero until a peak earns a ratchet. Action/Reason are the live
verdict for journals and UI.
*/
type Stoploss struct {
	sync.RWMutex `json:"-"`
	cancel       context.CancelFunc
	armed        bool
	entry        float64
	epoch        uint64

	Weight        float64 `json:"weight"`
	LockedFloor   float64 `json:"lockedFloor"`
	FloorDistance float64 `json:"floorDistance"`
	TrailDistance float64 `json:"trailDistance"`
	StopReturn    float64 `json:"stopReturn"`
	PeakReturn    float64 `json:"peakReturn"`
	MarkReturn    float64 `json:"markReturn"`
	Retreat       float64 `json:"retreat"`
	Action        string  `json:"action"`
	Reason        string  `json:"reason"`
}

/*
NewStoploss constructs an unbound regulator. Bind arms it at fill.
*/
func NewStoploss(ctx context.Context) *Stoploss {
	_, cancel := context.WithCancel(ctx)

	return &Stoploss{
		cancel: cancel,
		Weight: 1,
		Action: "hold",
		Reason: "unbound",
	}
}

/*
Bind arms the stop at entry with an initial adverse band (fee/spread width).
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
	stoploss.LockedFloor = 0
	stoploss.PeakReturn = 0
	stoploss.MarkReturn = 0
	stoploss.FloorDistance = distance
	stoploss.TrailDistance = distance
	stoploss.StopReturn = -distance
	stoploss.Action = "hold"
	stoploss.Reason = "bound at entry"
}

/*
Restore arms from durable recovery state without resetting ratchet geometry.
*/
func (stoploss *Stoploss) Restore(entry float64, recovered *Stoploss) {
	if stoploss == nil || recovered == nil || entry <= 0 {
		return
	}

	stoploss.armed = true
	stoploss.entry = entry
	stoploss.epoch = recovered.epoch
	stoploss.Weight = recovered.Weight
	stoploss.LockedFloor = recovered.LockedFloor
	stoploss.FloorDistance = recovered.FloorDistance
	stoploss.TrailDistance = recovered.TrailDistance
	stoploss.StopReturn = recovered.StopReturn
	stoploss.PeakReturn = recovered.PeakReturn
	stoploss.MarkReturn = recovered.MarkReturn
	stoploss.Retreat = recovered.Retreat
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
Armed reports whether entry and a live floor are latched.
*/
func (stoploss *Stoploss) Armed() bool {
	if stoploss == nil {
		return false
	}

	stoploss.RLock()
	defer stoploss.RUnlock()

	return stoploss.armed
}

/*
StopPrice is the absolute floor: entry×(1+stopReturn).
*/
func (stoploss *Stoploss) StopPrice() float64 {
	if stoploss == nil {
		return 0
	}

	stoploss.RLock()
	defer stoploss.RUnlock()

	if !stoploss.armed || stoploss.entry <= 0 {
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
WidenSurvival raises the unlocked fill-time band when live width exceeds it.
*/
func (stoploss *Stoploss) WidenSurvival(distance float64) {
	if stoploss == nil {
		return
	}

	stoploss.Lock()
	defer stoploss.Unlock()

	if !stoploss.armed {
		return
	}

	if distance <= 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
		return
	}

	if stoploss.LockedFloor > 0 {
		return
	}

	if distance <= stoploss.FloorDistance {
		return
	}

	stoploss.FloorDistance = distance

	if stoploss.TrailDistance < distance {
		stoploss.TrailDistance = distance
	}

	next := -stoploss.TrailDistance

	if stoploss.PeakReturn > 0 {
		next = math.Max(stoploss.StopReturn, next)
	}

	stoploss.StopReturn = next
	stoploss.Reason = "survival band widened from live entry trail"
}

/*
Update applies one evidence cut: arm if needed, rescale the band, ratchet the
floor, and set stop / take_profit / hold.
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
			distance = liveWidth(evidence)
		}

		stoploss.Bind(evidence.Entry, distance)
		stoploss.seedWeight(evidence)
	}

	stoploss.heal(evidence)

	if evidence.RetreatReady {
		stoploss.Retreat = math.Max(0, evidence.RetreatPressure)
	}

	markReturn := (evidence.Mark - stoploss.entry) / stoploss.entry

	// Retreat freezes geometry so a spoofed touch withdrawal cannot fake a stop
	// out — but only while the forward path is not itself adverse. A calibrated
	// negative forecast (or causal decline) means the mark fell because price is
	// genuinely leaving, not because liquidity blinked, so the protective stop
	// must remain able to fire through the withdrawal instead of freezing open.
	adverseForward := evidence.ReturnReady && evidence.ExpectedReturn < 0 ||
		evidence.CausalReady && evidence.CausalExpectedReturn < 0

	if stoploss.Retreat > 0 && !adverseForward {
		return stoploss.settle("hold", "retreat-driven mark; geometry frozen", markReturn)
	}

	if !stoploss.armed {
		return stoploss.settle("hold", "stop unbound; trail width absent", markReturn)
	}

	stoploss.reweight(evidence)
	stoploss.rescale(evidence)
	stoploss.ratchet(markReturn)

	if stoploss.breached(markReturn) {
		return stoploss.settle("stop", "mark returned through live stop", markReturn)
	}

	if stoploss.takeProfit(evidence, markReturn) {
		return stoploss.settle(
			"take_profit",
			"peak proximity with non-positive forward path",
			markReturn,
		)
	}

	return stoploss.settle("hold", "stop live; path intact", markReturn)
}

/*
Regulate appends an exit Decision when Update fires stop or take_profit.
*/
func (stoploss *Stoploss) Regulate(
	thesis *Thesis,
	holding Holding,
	evidence StopEvidence,
) {
	if stoploss == nil || thesis == nil || holding.Symbol == "" {
		return
	}

	stoploss.Lock()
	defer stoploss.Unlock()

	if phase, found := thesis.Lifecycle.Load(holding.Symbol); found {
		if phase == LifecycleExitSelected || phase == LifecycleExitSubmitted {
			return
		}
	}

	stoploss.Update(evidence)

	if stoploss.Action != "stop" && stoploss.Action != "take_profit" {
		return
	}

	if evidence.ReferencePrice == nil || evidence.ReferencePrice.Sign() <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"stoploss: exact reference price required for exit decision",
			nil,
		))

		return
	}

	thesis.Decisions = append(thesis.Decisions, Decision{
		Action:           "exit",
		Symbol:           holding.Symbol,
		At:               thesis.At,
		Utility:          stoploss.StopReturn,
		Alternatives:     map[string]float64{stoploss.Action: stoploss.StopReturn},
		ProposedQuantity: holding.Qty,
		ReferencePrice:   evidence.ReferencePrice.Copy(),
		Cause:            stoploss.Action,
		Reason:           stoploss.Reason,
	})
	thesis.NoteLifecycle(holding.Symbol, LifecycleExitSelected, thesis.At)
}

/*
ObserveMark ratchets from a live mark. It never emits exit Actions — Regulate
does that on an evidence cut.
*/
func (stoploss *Stoploss) ObserveMark(mark float64) {
	if stoploss == nil || !stoploss.armed || stoploss.entry <= 0 || mark <= 0 {
		return
	}

	markReturn := (mark - stoploss.entry) / stoploss.entry
	stoploss.MarkReturn = markReturn
	stoploss.Action = "hold"

	if stoploss.Retreat > 0 {
		stoploss.Reason = "retreat-gated mark; await sincere print"
		return
	}

	stoploss.ratchet(markReturn)
	stoploss.Reason = "stop live; path intact"

	if stoploss.breached(markReturn) {
		stoploss.Reason = "mark breached live stop; await regulate"
	}
}

/*
NoteRetreat latches toxicity retreat so tick marks share Update's sincerity gate.
*/
func (stoploss *Stoploss) NoteRetreat(pressure float64) {
	if stoploss == nil {
		return
	}

	stoploss.Lock()
	defer stoploss.Unlock()
	stoploss.Retreat = math.Max(0, pressure)
}

func (stoploss *Stoploss) settle(action, reason string, markReturn float64) *Stoploss {
	stoploss.Action = action
	stoploss.Reason = reason
	stoploss.MarkReturn = markReturn

	return stoploss
}

func (stoploss *Stoploss) heal(evidence StopEvidence) {
	if !stoploss.armed {
		return
	}

	if stoploss.TrailDistance > 0 &&
		!math.IsNaN(stoploss.TrailDistance) &&
		!math.IsInf(stoploss.TrailDistance, 0) {
		return
	}

	distance := liveWidth(evidence)

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

func (stoploss *Stoploss) seedWeight(evidence StopEvidence) {
	if evidence.Uncertainty > 0 {
		magnitude := math.Abs(evidence.ExpectedReturn) + evidence.Uncertainty
		share := math.Abs(evidence.ExpectedReturn) / magnitude

		if share <= 0 {
			stoploss.Weight = 1
			return
		}

		stoploss.Weight = share
		return
	}

	if evidence.CognitionReady &&
		evidence.CognitionConfidence > 0 &&
		evidence.CognitionConfidence <= 1 {
		stoploss.Weight = evidence.CognitionConfidence
		return
	}

	stoploss.Weight = 1
}

func (stoploss *Stoploss) reweight(evidence StopEvidence) {
	if !evidence.ReturnReady || evidence.ForecastEpoch == stoploss.epoch {
		return
	}

	stoploss.epoch = evidence.ForecastEpoch
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

func (stoploss *Stoploss) rescale(evidence StopEvidence) {
	scale := liveWidth(evidence)

	if scale <= 0 || stoploss.Weight <= 0 ||
		math.IsNaN(stoploss.Weight) || math.IsInf(stoploss.Weight, 0) {
		return
	}

	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return
	}

	if evidence.Spread > scale {
		scale = evidence.Spread
	}

	next := scale / stoploss.Weight

	if math.IsNaN(next) || math.IsInf(next, 0) || next <= 0 {
		return
	}

	if stoploss.FloorDistance > 0 && next < stoploss.FloorDistance {
		next = stoploss.FloorDistance
	}

	stoploss.TrailDistance = next
}

func (stoploss *Stoploss) ratchet(markReturn float64) {
	if stoploss.TrailDistance <= 0 || math.IsNaN(stoploss.TrailDistance) ||
		math.IsInf(stoploss.TrailDistance, 0) {
		return
	}

	stoploss.MarkReturn = markReturn
	stoploss.PeakReturn = math.Max(stoploss.PeakReturn, markReturn)

	raised := stoploss.PeakReturn - stoploss.TrailDistance
	next := math.Max(-stoploss.TrailDistance, raised)
	survival := stoploss.FloorDistance

	if survival <= 0 {
		survival = stoploss.TrailDistance
	}

	if stoploss.PeakReturn > survival {
		candidate := stoploss.PeakReturn - stoploss.TrailDistance

		if candidate > 0 {
			stoploss.LockedFloor = math.Max(stoploss.LockedFloor, candidate)
		}
	}

	if stoploss.LockedFloor > 0 {
		next = math.Max(stoploss.LockedFloor, markReturn-stoploss.TrailDistance)
	}

	stoploss.StopReturn = math.Max(stoploss.StopReturn, next)
}

func (stoploss *Stoploss) breached(markReturn float64) bool {
	return stoploss.TrailDistance > 0 && markReturn <= stoploss.StopReturn
}

func (stoploss *Stoploss) takeProfit(evidence StopEvidence, markReturn float64) bool {
	if stoploss.PeakReturn <= 0 {
		return false
	}

	band := liveWidth(evidence)

	if evidence.Spread > band {
		band = evidence.Spread
	}

	if stoploss.PeakReturn-markReturn > band {
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

func liveWidth(evidence StopEvidence) float64 {
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
