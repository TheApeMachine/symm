package broker

import "time"

/*
LearningStop is the probation stop: same invalidation + trail shape as the
old active StopLoss, but seeded from AdaptiveRails and never settles a lot.
When its shadow episode ends, AdaptiveRails.Learn compares shadow vs official.
*/
type LearningStop struct {
	stop     *StopLoss
	rails    *AdaptiveRails
	snapshot RailsSnapshot
	armed    bool
	done     bool
	learned  bool
	trigger  string
	exitPct  float64

	Active      bool       `json:"active,omitempty"`
	PnLTrace    []float64  `json:"pnl_trace,omitempty"`
	ExitTrigger string     `json:"exit_trigger,omitempty"`
	ExitAt      *time.Time `json:"exit_at,omitempty"`
	ExitPct     float64    `json:"exit_pct,omitempty"`
	FloorPct    float64    `json:"floor_pct,omitempty"`
	FloorTrace  []float64  `json:"floor_trace,omitempty"`
	Proximity   float64    `json:"proximity,omitempty"`
	RoomPct     float64    `json:"room_pct,omitempty"`
}

func NewLearningStop(rails *AdaptiveRails) *LearningStop {
	return &LearningStop{
		stop:   NewStopLoss(),
		rails:  rails,
		Active: true,
	}
}

/*
Arm freezes a rails snapshot and friction-aware budget for this episode.
*/
func (learning *LearningStop) Arm(frictionFraction float64) {
	if learning.armed || learning.done {
		return
	}

	learning.armed = true
	learning.snapshot = learning.rails.Snapshot()

	budget := NewLossBudget().Arm(frictionFraction)
	budget.Prefer(learning.snapshot.MaxLoss, learning.snapshot.ArmPct)

	learning.stop.budget = budget
	learning.stop.baseGiveback = learning.snapshot.BaseGiveback
	learning.stop.minGiveback = learning.snapshot.MinGiveback
}

/*
Live reports whether the shadow still wants marks.
*/
func (learning *LearningStop) Live() bool {
	return learning.Active && !learning.done
}

/*
Update advances the shadow stop and keeps its own traces/floors.
ok means the learning episode would exit.
*/
func (learning *LearningStop) Update(currentPnL float64) (trigger string, ok bool) {
	if learning.done {
		return "", false
	}

	learning.PnLTrace = appendTrace(learning.PnLTrace, currentPnL)

	trigger, ok = learning.stop.Update(currentPnL)
	learning.refresh(currentPnL)

	if !ok {
		return "", false
	}

	learning.close(trigger, currentPnL)
	return trigger, true
}

/*
Finish records a shadow exit when the episode ends outside Update.
*/
func (learning *LearningStop) Finish(trigger string, exitPct float64) {
	if learning.done {
		return
	}

	learning.close(trigger, exitPct)
}

/*
Settle learns from the official structural exit once the shadow finished.
*/
func (learning *LearningStop) Settle(officialPct float64) {
	if !learning.done || learning.learned {
		return
	}

	learning.learned = true
	learning.rails.Learn(officialPct, learning.exitPct)
}

func (learning *LearningStop) refresh(currentPnL float64) {
	learning.FloorPct = learning.stop.ActiveFloor(currentPnL)
	learning.RoomPct = currentPnL - learning.FloorPct
	learning.Proximity = learning.stop.Proximity(currentPnL)
	learning.FloorTrace = appendTrace(learning.FloorTrace, learning.FloorPct)
}

func (learning *LearningStop) close(trigger string, exitPct float64) {
	now := time.Now().UTC()

	learning.done = true
	learning.Active = false
	learning.trigger = trigger
	learning.ExitTrigger = trigger
	learning.exitPct = exitPct
	learning.ExitPct = exitPct
	learning.ExitAt = &now
}
