package stoploss

import (
	"fmt"
	"math"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/system"
)

const (
	structuralPhaseIdle     = ""
	structuralPhasePeak     = "peak"
	structuralPhasePullback = "pullback"
	structuralPhaseBounce   = "bounce"
)

/*
Structural is the active settler: milestone floors plus failed-retest
exhaustion. Position only Arms, Updates, and reacts when Update fires.
*/
type Structural struct {
	armed       bool
	initialized bool
	done        bool
	entryPnL    float64
	peakPnL     float64
	floor       float64
	milestone   int
	phase       string
	trough      float64
	bounceHigh  float64
	priorPeak   float64
	trigger     string
	budget      *Budget

	FloorPct   float64 `json:"floor_pct,omitempty"`
	RoomPct    float64 `json:"room_pct,omitempty"`
	Proximity  float64 `json:"proximity,omitempty"`
	PnLTrace   Path    `json:"pnl_trace,omitempty"`
	FloorTrace Path    `json:"floor_trace,omitempty"`
	State      State   `json:"state,omitempty"`
}

/*
Machine is the locked risk rails that must survive a restart. Phase /
pullback / bounce are intentionally omitted — a blackout drops the
retest narrative and reseeds from peak on Resume.
*/
type Machine struct {
	Armed       bool    `json:"armed"`
	Initialized bool    `json:"initialized"`
	Done        bool    `json:"done"`
	EntryPnL    float64 `json:"entry_pnl"`
	PeakPnL     float64 `json:"peak_pnl"`
	Floor       float64 `json:"floor"`
	Milestone   int     `json:"milestone"`
	Trigger     string  `json:"trigger,omitempty"`
	MaxLoss     float64 `json:"max_loss"`
	ArmPct      float64 `json:"arm_pct"`
}

func NewStructural() *Structural {
	return &Structural{
		budget: NewBudget().Arm(0),
		phase:  structuralPhaseIdle,
	}
}

// Arm freezes zone-0 from entry friction. Only lock once friction is known.
func (stop *Structural) Arm(frictionFraction float64) {
	if stop.armed || frictionFraction <= 0 {
		return
	}

	stop.budget = NewBudget().Arm(frictionFraction)
	stop.armed = true

	// Widen provisional invalidation if Update ran before real friction.
	if stop.initialized && stop.milestone == 0 {
		stop.floor = math.Min(stop.floor, -stop.budget.maxLoss)
	}
}

/*
Resume reloads locked rails after a restart. Mid-phase pullback/bounce
is discarded so an unobserved gap cannot finish a retest that never
played out live; peak and floor stay. Retest stays idle until peak
has cleared the arm hurdle.
*/
func (stop *Structural) Resume(machine Machine) {
	if !machine.Initialized {
		return
	}

	if stop.budget == nil {
		stop.budget = NewBudget().Arm(0)
	}

	stop.armed = machine.Armed
	stop.initialized = true
	stop.done = machine.Done
	stop.entryPnL = machine.EntryPnL
	stop.peakPnL = machine.PeakPnL
	stop.floor = machine.Floor
	stop.milestone = machine.Milestone
	stop.trigger = machine.Trigger
	stop.budget.maxLoss = machine.MaxLoss
	stop.budget.armPct = machine.ArmPct
	stop.budget.armed = machine.Armed || machine.MaxLoss > 0

	stop.priorPeak = machine.PeakPnL
	stop.trough = machine.PeakPnL
	stop.bounceHigh = machine.PeakPnL
	stop.phase = structuralPhaseIdle

	if stop.peakPnL > stop.budget.armPct {
		stop.phase = structuralPhasePeak
	}

	stop.FloorPct = machine.Floor
	stop.State.PeakPct = machine.PeakPnL
	stop.State.FloorPct = machine.Floor
	stop.State.MaxLossPct = machine.MaxLoss
}

func (stop *Structural) Machine() Machine {
	maxLoss, armPct := 0.0, 0.0

	if stop.budget != nil {
		maxLoss = stop.budget.maxLoss
		armPct = stop.budget.armPct
	}

	return Machine{
		Armed:       stop.armed,
		Initialized: stop.initialized,
		Done:        stop.done,
		EntryPnL:    stop.entryPnL,
		PeakPnL:     stop.peakPnL,
		Floor:       stop.floor,
		Milestone:   stop.milestone,
		Trigger:     stop.trigger,
		MaxLoss:     maxLoss,
		ArmPct:      armPct,
	}
}

type structuralJSON struct {
	Machine    Machine `json:"machine"`
	FloorPct   float64 `json:"floor_pct,omitempty"`
	RoomPct    float64 `json:"room_pct,omitempty"`
	Proximity  float64 `json:"proximity,omitempty"`
	PnLTrace   Path    `json:"pnl_trace,omitempty"`
	FloorTrace Path    `json:"floor_trace,omitempty"`
	State      State   `json:"state,omitempty"`
}

func (stop Structural) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(structuralJSON{
		Machine:    stop.Machine(),
		FloorPct:   stop.FloorPct,
		RoomPct:    stop.RoomPct,
		Proximity:  stop.Proximity,
		PnLTrace:   stop.PnLTrace,
		FloorTrace: stop.FloorTrace,
		State:      stop.State,
	})
}

func (stop *Structural) UnmarshalJSON(raw []byte) error {
	var view structuralJSON

	if err := sonic.Unmarshal(raw, &view); err != nil {
		return err
	}

	if stop.budget == nil {
		stop.budget = NewBudget().Arm(0)
	}

	stop.FloorPct = view.FloorPct
	stop.RoomPct = view.RoomPct
	stop.Proximity = view.Proximity
	stop.PnLTrace = view.PnLTrace
	stop.FloorTrace = view.FloorTrace
	stop.State = view.State

	if view.Machine.Initialized {
		stop.Resume(view.Machine)
	}

	return nil
}

// Update advances structural state and refreshes the live floor view.
func (stop *Structural) Update(currentPnL float64) (string, bool) {
	defer stop.refresh(currentPnL)

	if stop.done {
		return "", false
	}

	stop.PnLTrace.Mark(currentPnL)

	if !stop.initialized {
		// PnL% is already vs entry fill — anchor rails at 0 so a resume
		// without saved machine still invalidates from true entry, not
		// from whatever mark happens to arrive first after boot.
		stop.initialized = true
		stop.entryPnL = 0
		stop.floor = -stop.budget.maxLoss
		stop.peakPnL = currentPnL
		stop.priorPeak = currentPnL
		stop.trough = currentPnL
		stop.bounceHigh = currentPnL
		stop.phase = structuralPhaseIdle
		return "", false
	}

	if currentPnL <= stop.floor {
		return stop.finish(fmt.Sprintf(
			"structural floor: PnL %.2f%% hit floor %.2f%% (peak %.2f%%, milestone %d)",
			currentPnL, stop.floor, stop.peakPnL, stop.milestone,
		))
	}

	if currentPnL > stop.peakPnL {
		stop.peakPnL = currentPnL
		stop.ratchetMilestone()
	}

	// Retest exhaustion only after the lot has cleared friction and is green.
	if stop.peakPnL <= stop.budget.armPct || currentPnL <= 0 {
		stop.phase = structuralPhaseIdle
		return "", false
	}

	// Scale noise with peak so massive runners aren't clipped by fixed ticks.
	baseNoise := system.Cfg.Risk.StructuralPullbackPct
	noise := math.Max(baseNoise, stop.peakPnL*0.15)

	switch stop.phase {
	case structuralPhaseIdle:
		stop.setPeak(currentPnL)

	case structuralPhasePeak:
		if currentPnL > stop.peakPnL {
			stop.setPeak(currentPnL)
		} else if stop.peakPnL-currentPnL >= noise {
			stop.phase = structuralPhasePullback
			stop.trough = currentPnL
		}

	case structuralPhasePullback:
		stop.trough = math.Min(stop.trough, currentPnL)

		if currentPnL > stop.trough+noise*0.35 {
			stop.phase = structuralPhaseBounce
			stop.bounceHigh = currentPnL
		}

	case structuralPhaseBounce:
		stop.bounceHigh = math.Max(stop.bounceHigh, currentPnL)

		if currentPnL > stop.priorPeak {
			if stop.trough > 0 {
				stop.floor = math.Max(stop.floor, stop.trough)
			}

			stop.setPeak(currentPnL)
			return "", false
		}

		mid := (stop.trough + stop.bounceHigh) * 0.5
		missed := stop.priorPeak - stop.bounceHigh
		bounce := stop.bounceHigh - stop.trough

		if missed >= noise && bounce >= noise*0.5 && currentPnL < mid {
			return stop.finish(fmt.Sprintf(
				"structural exhaustion: failed retest of %.2f%% (bounce high %.2f%%, mid %.2f%%, current %.2f%%)",
				stop.priorPeak, stop.bounceHigh, mid, currentPnL,
			))
		}

		if currentPnL < stop.trough {
			stop.phase = structuralPhasePullback
			stop.trough = currentPnL
		}
	}

	return "", false
}

func (stop *Structural) setPeak(pnl float64) {
	stop.peakPnL = pnl
	stop.priorPeak = pnl
	stop.trough = pnl
	stop.bounceHigh = pnl
	stop.phase = structuralPhasePeak
	stop.ratchetMilestone()
}

func (stop *Structural) ratchetMilestone() {
	r := system.Cfg.Risk
	tiers := [...]struct {
		level         int
		target, floor float64
	}{
		{3, r.StructuralMilestone3Pct, r.StructuralFloor3Pct},
		{2, r.StructuralMilestone2Pct, r.StructuralFloor2Pct},
		{1, r.StructuralMilestone1Pct, r.StructuralFloor1Pct},
	}

	for _, t := range tiers {
		if stop.peakPnL >= t.target && stop.milestone < t.level {
			stop.milestone = t.level
			stop.floor = math.Max(stop.floor, t.floor)
			break
		}
	}
}

func (stop *Structural) refresh(currentPnL float64) {
	stop.FloorPct = stop.floor
	stop.RoomPct = currentPnL - stop.floor
	stop.Proximity = math.Max(0, math.Min(1, 1-stop.RoomPct/2.0))
	stop.FloorTrace.Mark(stop.floor)
	stop.State = State{
		CurrentPct: currentPnL,
		PeakPct:    stop.peakPnL,
		FloorPct:   stop.FloorPct,
		RoomPct:    stop.RoomPct,
		Proximity:  stop.Proximity,
		MaxLossPct: stop.budget.maxLoss,
	}
}

func (stop *Structural) finish(trigger string) (string, bool) {
	stop.done = true
	stop.trigger = trigger
	return trigger, true
}
