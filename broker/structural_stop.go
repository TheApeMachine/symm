package broker

import (
	"fmt"

	"github.com/theapemachine/symm/system"
)

const (
	structuralPhasePeak     = "peak"
	structuralPhasePullback = "pullback"
	structuralPhaseBounce   = "bounce"
)

/*
StructuralStop is the active settler: milestone floors plus failed-retest
exhaustion. Position only Arms, Updates, and reacts when Update fires.
*/
type StructuralStop struct {
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
	budget      *LossBudget

	FloorPct   float64   `json:"floor_pct,omitempty"`
	RoomPct    float64   `json:"room_pct,omitempty"`
	Proximity  float64   `json:"proximity,omitempty"`
	FloorTrace []float64 `json:"floor_trace,omitempty"`
	State      StopState `json:"state,omitempty"`
}

func NewStructuralStop() *StructuralStop {
	return &StructuralStop{budget: NewLossBudget().Arm(0)}
}

/*
Arm freezes zone-0 from entry friction once. Later calls are no-ops.
*/
func (stop *StructuralStop) Arm(frictionFraction float64) {
	if stop.armed {
		return
	}

	stop.armed = true

	if frictionFraction > 0 {
		stop.budget = NewLossBudget().Arm(frictionFraction)
	}
}

/*
Update advances structural state and refreshes the live floor view.
ok means the active lot should settle.
*/
func (stop *StructuralStop) Update(currentPnL float64) (trigger string, ok bool) {
	if stop.done {
		stop.refresh(currentPnL)
		return "", false
	}

	if !stop.initialized {
		stop.initialized = true
		stop.entryPnL = currentPnL
		stop.peakPnL = currentPnL
		stop.floor = stop.entryPnL - stop.budget.maxLoss
		stop.phase = structuralPhasePeak
		stop.priorPeak = currentPnL
		stop.trough = currentPnL
		stop.bounceHigh = currentPnL
		stop.refresh(currentPnL)
		return "", false
	}

	stop.ratchetMilestone(currentPnL)

	if currentPnL <= stop.floor {
		trigger, ok = stop.finish(fmt.Sprintf(
			"structural floor: PnL %.2f%% hit floor %.2f%% (peak %.2f%%, milestone %d)",
			currentPnL, stop.floor, stop.peakPnL, stop.milestone,
		))
		stop.refresh(currentPnL)
		return trigger, ok
	}

	noise := system.Cfg.Risk.StructuralPullbackPct

	switch stop.phase {
	case structuralPhasePeak:
		if currentPnL > stop.peakPnL {
			stop.peakPnL = currentPnL
			stop.priorPeak = currentPnL
			stop.trough = currentPnL
			stop.ratchetMilestone(currentPnL)
			stop.refresh(currentPnL)
			return "", false
		}

		if stop.peakPnL-currentPnL >= noise {
			stop.phase = structuralPhasePullback
			stop.trough = currentPnL
			stop.priorPeak = stop.peakPnL
		}

	case structuralPhasePullback:
		if currentPnL < stop.trough {
			stop.trough = currentPnL
		}

		if currentPnL > stop.trough+noise*0.25 {
			stop.phase = structuralPhaseBounce
			stop.bounceHigh = currentPnL
		}

	case structuralPhaseBounce:
		if currentPnL > stop.bounceHigh {
			stop.bounceHigh = currentPnL
		}

		if currentPnL > stop.priorPeak {
			stop.peakPnL = currentPnL
			stop.priorPeak = currentPnL
			stop.phase = structuralPhasePeak
			stop.trough = currentPnL
			stop.bounceHigh = currentPnL
			stop.liftFloorToTrough()
			stop.ratchetMilestone(currentPnL)
			stop.refresh(currentPnL)
			return "", false
		}

		mid := stop.trough + (stop.bounceHigh-stop.trough)*0.5

		if stop.bounceHigh < stop.priorPeak && currentPnL < mid &&
			stop.bounceHigh-stop.trough >= noise*0.5 {
			trigger, ok = stop.finish(fmt.Sprintf(
				"structural exhaustion: failed retest of %.2f%% (bounce high %.2f%%, mid %.2f%%, current %.2f%%)",
				stop.priorPeak, stop.bounceHigh, mid, currentPnL,
			))
			stop.refresh(currentPnL)
			return trigger, ok
		}

		if currentPnL < stop.trough {
			stop.phase = structuralPhasePullback
			stop.trough = currentPnL
		}
	}

	stop.refresh(currentPnL)
	return "", false
}

func (stop *StructuralStop) refresh(currentPnL float64) {
	stop.FloorPct = stop.floor
	stop.RoomPct = currentPnL - stop.floor
	stop.Proximity = structuralProximity(currentPnL, stop.floor)
	stop.FloorTrace = appendTrace(stop.FloorTrace, stop.floor)
	stop.State = StopState{
		CurrentPct: currentPnL,
		PeakPct:    stop.peakPnL,
		FloorPct:   stop.FloorPct,
		RoomPct:    stop.RoomPct,
		Proximity:  stop.Proximity,
		MaxLossPct: stop.budget.maxLoss,
	}
}

func (stop *StructuralStop) finish(trigger string) (string, bool) {
	stop.done = true
	stop.trigger = trigger
	return trigger, true
}

func (stop *StructuralStop) ratchetMilestone(currentPnL float64) {
	peak := stop.peakPnL
	if currentPnL > peak {
		peak = currentPnL
		stop.peakPnL = peak
	}

	risk := system.Cfg.Risk

	switch {
	case peak >= risk.StructuralMilestone3Pct && stop.milestone < 3:
		stop.milestone = 3
		stop.floor = maxFloat(stop.floor, risk.StructuralFloor3Pct)
	case peak >= risk.StructuralMilestone2Pct && stop.milestone < 2:
		stop.milestone = 2
		stop.floor = maxFloat(stop.floor, risk.StructuralFloor2Pct)
	case peak >= risk.StructuralMilestone1Pct && stop.milestone < 1:
		stop.milestone = 1
		stop.floor = maxFloat(stop.floor, risk.StructuralFloor1Pct)
	}
}

func (stop *StructuralStop) liftFloorToTrough() {
	if stop.trough > stop.floor {
		stop.floor = stop.trough
	}
}

func structuralProximity(current, floor float64) float64 {
	room := current - floor
	if room <= 0 {
		return 1
	}

	const span = 2.0
	if room >= span {
		return 0
	}

	return 1 - room/span
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}

	return b
}
