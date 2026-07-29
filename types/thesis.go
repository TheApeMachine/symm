package types

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
)

const (
	ThesisKey = "thesis"

	LifecycleObserving           = "observing"
	LifecycleShaped              = "shaped"
	LifecycleEntrySelected       = "entry_selected"
	LifecycleEntrySubmitted      = "entry_submitted"
	LifecyclePartiallyEntered    = "partially_entered"
	LifecycleEntered             = "entered"
	LifecycleManaging            = "managing"
	LifecycleExitSelected        = "exit_selected"
	LifecycleExitSubmitted       = "exit_submitted"
	LifecyclePartiallyExited     = "partially_exited"
	LifecycleClosed              = "closed"
	LifecyclePostExitObservation = "post_exit_observation"
	LifecyclePostMortemReady     = "postmortem_ready"
	LifecycleEvaluated           = "evaluated"
	LifecycleExpired             = "expired"
	LifecycleRejected            = "rejected"
	LifecycleInvalid             = "invalid"
)

/*
Thesis is the durable lifecycle record from entry through post-mortem. It keeps
decisions, lifecycle, and findings; each market cut replaces per-tick evidence
in place so the object does not grow without bound.
*/
type Thesis struct {
	checkpoint    atomic.Int64
	publish       *sync.RWMutex
	Tick          int64                 `json:"tick"`
	At            time.Time             `json:"at"`
	CrossSection  *CrossSection         `json:"crossSection"`
	Measurements  *sync.Map             `json:"-"`
	BookManager   *spot.BookManager     `json:"-"`
	Books         *sync.Map             `json:"-"`
	Graphs        *sync.Map             `json:"-"`
	Forecasts     []Forecasts           `json:"forecasts"`
	Decisions     []Decision            `json:"decisions"`
	Lifecycle     *sync.Map             `json:"lifecycle"`
	Findings      []Finding             `json:"findings"`
	Hypotheses    []Hypothesis          `json:"hypotheses"`
	Categories    map[string][]Category `json:"categories"`
	Manifold      *sync.Map             `json:"-"`
	Cognition     *sync.Map             `json:"-"`
	Resonance     *sync.Map             `json:"-"`
	Causal        *sync.Map             `json:"-"`
	cutIncomplete bool
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis() *Thesis {
	return &Thesis{
		At:           time.Now().UTC(),
		publish:      &sync.RWMutex{},
		Decisions:    make([]Decision, 0),
		CrossSection: NewCrossSection(),
		Graphs:       &sync.Map{},
		Forecasts:    make([]Forecasts, 0),
		Lifecycle:    &sync.Map{},
		Findings:     make([]Finding, 0),
		Hypotheses:   make([]Hypothesis, 0),
		Categories:   make(map[string][]Category),
		Measurements: &sync.Map{},
		Books:        &sync.Map{},
		Manifold:     &sync.Map{},
		Cognition:    &sync.Map{},
		Resonance:    &sync.Map{},
		Causal:       &sync.Map{},
	}
}

/*
ResetTick replaces per-tick evidence while preserving Lifecycle phases that
span orders (entry/exit submitted) and Findings owned by this Thesis.
*/
func (thesis *Thesis) ResetTick(at time.Time, tick int64) {
	thesis.ensureLocks()

	thesis.Tick = tick
	thesis.At = at

	thesis.CrossSection = NewCrossSection()
	thesis.Measurements.Clear()

	thesis.Forecasts = thesis.Forecasts[:0]
	thesis.Decisions = thesis.Decisions[:0]
	thesis.Hypotheses = thesis.Hypotheses[:0]

	for symbol, rows := range thesis.Categories {
		thesis.Categories[symbol] = rows[:0]
	}

	thesis.Resonance.Clear()
	thesis.Causal.Clear()
	thesis.Manifold.Clear()
	thesis.Cognition.Clear()
}

/*
StampAt sets Thesis.At to the latest measurement timestamp so Actor-driven cuts
carry market time instead of the wall clock captured at NewThesis.
*/
func (thesis *Thesis) StampAt() {
	thesis.ensureLocks()

	var latest time.Time
	thesis.Measurements.Range(func(_, value any) bool {
		row, _ := value.(*Measurement)

		if row == nil || row.At.IsZero() {
			return true
		}

		if row.At.After(latest) {
			latest = row.At
		}

		return true
	})

	if latest.IsZero() {
		return
	}

	thesis.At = latest
}

/*
AppendMeasurements appends published signal rows into the grouped Thesis surface
under the Thesis publish lock. Signal actors run independently, so direct map
writes can collide; this keeps the current grouped shape without restoring the
old Publish or snapshot APIs.
*/
func (thesis *Thesis) AppendMeasurements(rows []*Measurement) {
	thesis.ensureLocks()

	for _, row := range rows {
		if row == nil {
			continue
		}

		thesis.Measurements.Store(row.Key(), row)
	}
}

/*
ReplaceMeasurements swaps one symbol's measurement group under the shared Thesis
measurement lock. Analyzer observe output is an upsert, not an append, because a
manifold epoch replaces that symbol's derived evidence while signal actors may
still be publishing raw measurements on neighboring goroutines.
*/
func (thesis *Thesis) ReplaceMeasurements(symbol string, rows []*Measurement) {
	thesis.ensureLocks()
	thesis.Measurements.Range(func(key, value any) bool {
		measurement, ok := value.(*Measurement)

		if ok && measurement != nil && measurement.Symbol == symbol {
			thesis.Measurements.Delete(key)
		}

		return true
	})

	for _, row := range rows {
		if row != nil {
			thesis.Measurements.Store(row.Key(), row)
		}
	}
}

/*
EachMeasurement walks the grouped measurement surface under the Thesis publish
read lock. Readers use it while signal actors may append rows, preventing map
iteration races without reviving the removed snapshot allocation path.
*/
func (thesis *Thesis) EachMeasurement(yield func(*Measurement) bool) {
	thesis.ensureLocks()

	thesis.Measurements.Range(func(_, value any) bool {
		row, _ := value.(*Measurement)

		if row != nil && !yield(row) {
			return false
		}

		return true
	})
}

func (thesis *Thesis) cutMeasurements() map[string][]*Measurement {
	thesis.ensureLocks()

	frozen := make(map[string][]*Measurement)

	thesis.Measurements.Range(func(key, value any) bool {
		row, ok := value.(*Measurement)

		if !ok || row == nil {
			return true
		}

		frozen[row.Symbol] = append(frozen[row.Symbol], row)
		return true
	})

	return frozen
}

/*
cutMap freezes keyed per-symbol analyzer rows without turning the Thesis back
into append-only slices. The values remain shared immutable outcome pointers.
*/
func (thesis *Thesis) cutMap(rows *sync.Map) map[string]any {
	frozen := make(map[string]any)

	if rows == nil {
		return frozen
	}

	rows.Range(func(key, value any) bool {
		name, ok := key.(string)

		if ok && value != nil {
			frozen[name] = value
		}

		return true
	})

	return frozen
}

/*
ensureLocks restores pointer-owned locks after JSON decode or accidental value
construction. Thesis maps may be shared by copied values, so the lock itself must
also be shared rather than copied by value.
*/
func (thesis *Thesis) ensureLocks() {
	if thesis.publish == nil {
		thesis.publish = &sync.RWMutex{}
	}
}

/*
NoteIncomplete marks this cut as missing at least one interested signal measure.
*/
func (thesis *Thesis) NoteIncomplete() {
	if thesis != nil {
		thesis.cutIncomplete = true
	}
}

/*
Incomplete reports whether the current cut skipped interested signal work.
*/
func (thesis *Thesis) Incomplete() bool {
	return thesis != nil && thesis.cutIncomplete
}

/*
Save persists an immutable completed-cut snapshot under dir. The live Thesis is
not marshaled while mutable; callers must pass a finalized ImmutableCut.
*/
func (thesis *Thesis) Save(dir string, cut *ImmutableCut) error {
	if cut == nil {
		return fmt.Errorf("thesis: checkpoint requires ImmutableCut")
	}

	return cut.Checkpoint(dir)
}

/*
NoteLifecycle records a symbol phase transition. Desk inventory keeps its own
Holding.Status; this map is the strategy phase, not a second journal.
*/
func (thesis *Thesis) NoteLifecycle(symbol, status string, _ time.Time) {
	if thesis == nil || symbol == "" || status == "" {
		return
	}

	thesis.Lifecycle.Store(symbol, status)
}
