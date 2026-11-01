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
exhaustion. LearningStop is the evaluated shadow beside it.
*/
type StructuralStop struct {
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
}

func NewStructuralStop() *StructuralStop {
	return &StructuralStop{budget: NewLossBudget()}
}

/*
BindBudget shares the lot's frozen LossBudget for the zone-0 floor.
*/
func (stop *StructuralStop) BindBudget(budget *LossBudget) {
	if stop == nil || budget == nil {
		return
	}

	stop.budget = budget
}

func (stop *StructuralStop) Done() bool {
	return stop == nil || stop.done
}

func (stop *StructuralStop) Trigger() string {
	if stop == nil {
		return ""
	}

	return stop.trigger
}

func (stop *StructuralStop) Peak() float64 {
	if stop == nil {
		return 0
	}

	return stop.peakPnL
}

func (stop *StructuralStop) Floor() float64 {
	if stop == nil {
		return 0
	}

	return stop.floor
}

/*
Update advances structural state. ok means the active lot should settle.
*/
func (stop *StructuralStop) Update(currentPnL float64) (trigger string, ok bool) {
	if stop == nil || stop.done {
		return "", false
	}

	if !stop.initialized {
		stop.initialized = true
		stop.entryPnL = currentPnL
		stop.peakPnL = currentPnL
		stop.floor = stop.zone0Floor()
		stop.phase = structuralPhasePeak
		stop.priorPeak = currentPnL
		stop.trough = currentPnL
		stop.bounceHigh = currentPnL
		return "", false
	}

	stop.ratchetMilestone(currentPnL)

	if currentPnL <= stop.floor {
		return stop.finish(fmt.Sprintf(
			"structural floor: PnL %.2f%% hit floor %.2f%% (peak %.2f%%, milestone %d)",
			currentPnL, stop.floor, stop.peakPnL, stop.milestone,
		))
	}

	noise := structuralNoise()

	switch stop.phase {
	case structuralPhasePeak:
		if currentPnL > stop.peakPnL {
			stop.peakPnL = currentPnL
			stop.priorPeak = currentPnL
			stop.trough = currentPnL
			stop.ratchetMilestone(currentPnL)
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

		// Turn up from the trough starts a bounce attempt.
		if currentPnL > stop.trough+noise*0.25 {
			stop.phase = structuralPhaseBounce
			stop.bounceHigh = currentPnL
		}

	case structuralPhaseBounce:
		if currentPnL > stop.bounceHigh {
			stop.bounceHigh = currentPnL
		}

		if currentPnL > stop.priorPeak {
			// Higher high — trend confirmed; lift hard floor toward trough.
			stop.peakPnL = currentPnL
			stop.priorPeak = currentPnL
			stop.phase = structuralPhasePeak
			stop.trough = currentPnL
			stop.bounceHigh = currentPnL
			stop.liftFloorToTrough()
			stop.ratchetMilestone(currentPnL)
			return "", false
		}

		// Failed to take out the prior peak and rolled over mid-bounce.
		mid := stop.trough + (stop.bounceHigh-stop.trough)*0.5

		if stop.bounceHigh < stop.priorPeak && currentPnL < mid &&
			stop.bounceHigh-stop.trough >= noise*0.5 {
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

func (stop *StructuralStop) finish(trigger string) (string, bool) {
	stop.done = true
	stop.trigger = trigger
	return trigger, true
}

func (stop *StructuralStop) zone0Floor() float64 {
	if stop.budget == nil {
		stop.budget = NewLossBudget()
	}

	if !stop.budget.Armed() {
		stop.budget.Arm(0)
	}

	return stop.entryPnL - stop.budget.MaxLoss()
}

func (stop *StructuralStop) ratchetMilestone(currentPnL float64) {
	peak := stop.peakPnL
	if currentPnL > peak {
		peak = currentPnL
		stop.peakPnL = peak
	}

	m1, m2, m3 := structuralMilestones()
	f1, f2, f3 := structuralFloors()

	switch {
	case peak >= m3 && stop.milestone < 3:
		stop.milestone = 3
		stop.floor = maxFloat(stop.floor, f3)
	case peak >= m2 && stop.milestone < 2:
		stop.milestone = 2
		stop.floor = maxFloat(stop.floor, f2)
	case peak >= m1 && stop.milestone < 1:
		stop.milestone = 1
		stop.floor = maxFloat(stop.floor, f1)
	}
}

func (stop *StructuralStop) liftFloorToTrough() {
	// Never loosen a milestone floor; only ratchet up toward the swing trough.
	if stop.trough > stop.floor {
		stop.floor = stop.trough
	}
}

func structuralNoise() float64 {
	if system.Cfg != nil && system.Cfg.Risk != nil && system.Cfg.Risk.StructuralPullbackPct > 0 {
		return system.Cfg.Risk.StructuralPullbackPct
	}

	return 0.4
}

func structuralMilestones() (m1, m2, m3 float64) {
	m1, m2, m3 = 1.0, 2.0, 3.5
	if system.Cfg == nil || system.Cfg.Risk == nil {
		return
	}

	risk := system.Cfg.Risk
	if risk.StructuralMilestone1Pct > 0 {
		m1 = risk.StructuralMilestone1Pct
	}
	if risk.StructuralMilestone2Pct > 0 {
		m2 = risk.StructuralMilestone2Pct
	}
	if risk.StructuralMilestone3Pct > 0 {
		m3 = risk.StructuralMilestone3Pct
	}

	return
}

func structuralFloors() (f1, f2, f3 float64) {
	f1, f2, f3 = 0.35, 1.0, 2.2
	if system.Cfg == nil || system.Cfg.Risk == nil {
		return
	}

	risk := system.Cfg.Risk
	if risk.StructuralFloor1Pct != 0 {
		f1 = risk.StructuralFloor1Pct
	}
	if risk.StructuralFloor2Pct != 0 {
		f2 = risk.StructuralFloor2Pct
	}
	if risk.StructuralFloor3Pct != 0 {
		f3 = risk.StructuralFloor3Pct
	}

	return
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}

	return b
}
