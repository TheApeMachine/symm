package broker

/*
LearningStop is the probation stop: same invalidation + trail shape as the
old active StopLoss, but seeded from AdaptiveRails and never settles a lot.
When its shadow episode ends, AdaptiveRails.Learn compares shadow vs official.
*/
type LearningStop struct {
	stop     *StopLoss
	rails    *AdaptiveRails
	snapshot RailsSnapshot
	done     bool
	learned  bool
	trigger  string
	exitPct  float64
}

func NewLearningStop(rails *AdaptiveRails) *LearningStop {
	return &LearningStop{
		stop:  NewStopLoss(),
		rails: rails,
	}
}

/*
Arm freezes a rails snapshot and friction-aware budget for this episode.
*/
func (learning *LearningStop) Arm(frictionFraction float64) {
	if learning == nil || learning.done {
		return
	}

	if learning.stop == nil {
		learning.stop = NewStopLoss()
	}

	if learning.rails != nil {
		learning.snapshot = learning.rails.Snapshot()
	} else {
		learning.snapshot = NewAdaptiveRails().Snapshot()
	}

	budget := NewLossBudget().Arm(frictionFraction)
	budget.Prefer(learning.snapshot.MaxLoss, learning.snapshot.ArmPct)

	learning.stop.BindBudget(budget)
	learning.stop.BindGiveback(learning.snapshot.BaseGiveback, learning.snapshot.MinGiveback)
}

func (learning *LearningStop) Done() bool {
	return learning == nil || learning.done
}

func (learning *LearningStop) Trigger() string {
	if learning == nil {
		return ""
	}

	return learning.trigger
}

func (learning *LearningStop) ExitPct() float64 {
	if learning == nil {
		return 0
	}

	return learning.exitPct
}

func (learning *LearningStop) Snapshot() RailsSnapshot {
	if learning == nil {
		return RailsSnapshot{}
	}

	return learning.snapshot
}

/*
Update advances the shadow stop. ok means the learning episode would exit.
*/
func (learning *LearningStop) Update(currentPnL float64) (trigger string, ok bool) {
	if learning == nil || learning.done || learning.stop == nil {
		return "", false
	}

	trigger, ok = learning.stop.Update(currentPnL)
	if !ok {
		return "", false
	}

	learning.done = true
	learning.trigger = trigger
	learning.exitPct = currentPnL

	return trigger, true
}

func (learning *LearningStop) State(currentPnL float64) StopState {
	if learning == nil || learning.stop == nil {
		return StopState{CurrentPct: currentPnL}
	}

	return learning.stop.State(currentPnL)
}

func (learning *LearningStop) ActiveFloor(currentPnL float64) float64 {
	if learning == nil || learning.stop == nil {
		return 0
	}

	return learning.stop.ActiveFloor(currentPnL)
}

func (learning *LearningStop) Proximity(currentPnL float64) float64 {
	if learning == nil || learning.stop == nil {
		return 0
	}

	return learning.stop.Proximity(currentPnL)
}

/*
Finish records a shadow exit when the episode ends outside Update
(tests / forced close).
*/
func (learning *LearningStop) Finish(trigger string, exitPct float64) {
	if learning == nil || learning.done {
		return
	}

	learning.done = true
	learning.trigger = trigger
	learning.exitPct = exitPct
}

/*
Settle learns from the official structural exit when the shadow already
finished earlier, or when both end together.
*/
func (learning *LearningStop) Settle(officialPct float64) {
	if learning == nil || learning.rails == nil || !learning.done || learning.learned {
		return
	}

	learning.learned = true
	learning.rails.Learn(officialPct, learning.exitPct)
}
