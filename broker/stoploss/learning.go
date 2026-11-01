package stoploss

import (
	"time"

	"github.com/bytedance/sonic"
)

/*
Learning is the probation stop: same invalidation + trail shape as the
old active trail, but seeded from Rails and never settles a lot. When its
shadow episode ends, Rails.Learn compares shadow vs official.
*/
type Learning struct {
	stop     *StopLoss
	rails    *Rails
	snapshot Snapshot
	armed    bool
	done     bool
	learned  bool
	trigger  string
	exitPct  float64

	Active      bool       `json:"active,omitempty"`
	PnLTrace    Path       `json:"pnl_trace,omitempty"`
	ExitTrigger string     `json:"exit_trigger,omitempty"`
	ExitAt      *time.Time `json:"exit_at,omitempty"`
	ExitPct     float64    `json:"exit_pct,omitempty"`
	FloorPct    float64    `json:"floor_pct,omitempty"`
	FloorTrace  Path       `json:"floor_trace,omitempty"`
	Proximity   float64    `json:"proximity,omitempty"`
	RoomPct     float64    `json:"room_pct,omitempty"`
}

func NewLearning(rails *Rails) *Learning {
	return &Learning{
		stop:   NewStopLoss(),
		rails:  rails,
		Active: true,
	}
}

/*
Arm freezes a rails snapshot and friction-aware budget for this episode.
*/
func (learning *Learning) Arm(frictionFraction float64) {
	if learning.armed || learning.done {
		return
	}

	learning.armed = true
	learning.snapshot = learning.rails.Snapshot()

	budget := NewBudget().Arm(frictionFraction)
	budget.Prefer(learning.snapshot.MaxLoss, learning.snapshot.ArmPct)

	learning.stop.budget = budget
	learning.stop.baseGiveback = learning.snapshot.BaseGiveback
	learning.stop.minGiveback = learning.snapshot.MinGiveback
}

/*
Rebind attaches the desk-wide Rails after JSON restore. Trail extremes
already landed via UnmarshalJSON.
*/
func (learning *Learning) Rebind(rails *Rails) {
	learning.rails = rails

	if learning.stop == nil {
		learning.stop = NewStopLoss()
	}
}

func (learning *Learning) apply(view learningJSON) {
	learning.Active = view.Active
	learning.PnLTrace = view.PnLTrace
	learning.ExitTrigger = view.ExitTrigger
	learning.ExitAt = view.ExitAt
	learning.ExitPct = view.ExitPct
	learning.FloorPct = view.FloorPct
	learning.FloorTrace = view.FloorTrace
	learning.Proximity = view.Proximity
	learning.RoomPct = view.RoomPct
	learning.armed = view.Armed
	learning.done = view.Done
	learning.learned = view.Learned
	learning.trigger = view.ExitTrigger
	learning.exitPct = view.ExitPct
	learning.snapshot = view.Snapshot

	if learning.stop == nil {
		learning.stop = NewStopLoss()
	}

	if view.Trail.Initialized {
		learning.stop.Resume(view.Trail)
	}
}

type learningJSON struct {
	Active      bool         `json:"active,omitempty"`
	Armed       bool         `json:"armed,omitempty"`
	Done        bool         `json:"done,omitempty"`
	Learned     bool         `json:"learned,omitempty"`
	PnLTrace    Path         `json:"pnl_trace,omitempty"`
	ExitTrigger string       `json:"exit_trigger,omitempty"`
	ExitAt      *time.Time   `json:"exit_at,omitempty"`
	ExitPct     float64      `json:"exit_pct,omitempty"`
	FloorPct    float64      `json:"floor_pct,omitempty"`
	FloorTrace  Path         `json:"floor_trace,omitempty"`
	Proximity   float64      `json:"proximity,omitempty"`
	RoomPct     float64      `json:"room_pct,omitempty"`
	Snapshot    Snapshot     `json:"snapshot,omitempty"`
	Trail       TrailMachine `json:"trail,omitempty"`
}

func (learning Learning) MarshalJSON() ([]byte, error) {
	trail := TrailMachine{}

	if learning.stop != nil {
		trail = learning.stop.Machine()
	}

	return sonic.Marshal(learningJSON{
		Active:      learning.Active,
		Armed:       learning.armed,
		Done:        learning.done,
		Learned:     learning.learned,
		PnLTrace:    learning.PnLTrace,
		ExitTrigger: learning.ExitTrigger,
		ExitAt:      learning.ExitAt,
		ExitPct:     learning.ExitPct,
		FloorPct:    learning.FloorPct,
		FloorTrace:  learning.FloorTrace,
		Proximity:   learning.Proximity,
		RoomPct:     learning.RoomPct,
		Snapshot:    learning.snapshot,
		Trail:       trail,
	})
}

func (learning *Learning) UnmarshalJSON(raw []byte) error {
	var view learningJSON

	if err := sonic.Unmarshal(raw, &view); err != nil {
		return err
	}

	learning.apply(view)

	return nil
}

/*
Live reports whether the shadow still wants marks.
*/
func (learning *Learning) Live() bool {
	return learning.Active && !learning.done
}

/*
Update advances the shadow stop and keeps its own traces/floors.
ok means the learning episode would exit.
*/
func (learning *Learning) Update(currentPnL float64) (trigger string, ok bool) {
	if learning.done {
		return "", false
	}

	learning.PnLTrace.Mark(currentPnL)

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
func (learning *Learning) Finish(trigger string, exitPct float64) {
	if learning.done {
		return
	}

	learning.close(trigger, exitPct)
}

/*
Settle learns from the official structural exit once the shadow finished.
*/
func (learning *Learning) Settle(officialPct float64) {
	if !learning.done || learning.learned {
		return
	}

	learning.learned = true
	learning.rails.Learn(officialPct, learning.exitPct)
}

func (learning *Learning) refresh(currentPnL float64) {
	learning.FloorPct = learning.stop.ActiveFloor(currentPnL)
	learning.RoomPct = currentPnL - learning.FloorPct
	learning.Proximity = learning.stop.Proximity(currentPnL)
	learning.FloorTrace.Mark(learning.FloorPct)
}

func (learning *Learning) close(trigger string, exitPct float64) {
	now := time.Now().UTC()

	learning.done = true
	learning.Active = false
	learning.trigger = trigger
	learning.ExitTrigger = trigger
	learning.exitPct = exitPct
	learning.ExitPct = exitPct
	learning.ExitAt = &now
}
