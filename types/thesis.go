package types

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
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
holdings it created plus lifecycle and findings; each market cut replaces
per-tick evidence in place so the object does not grow without bound.
*/
type Thesis struct {
	checkpoint    atomic.Int64
	publish       sync.RWMutex
	Tick          int64                     `json:"tick"`
	At            time.Time                 `json:"at"`
	Positions     *sync.Map                 `json:"positions"`
	Holdings      *sync.Map                 `json:"holdings"`
	CrossSection  *CrossSection             `json:"crossSection"`
	Measurements  map[string][]*Measurement `json:"measurements"`
	Graphs        *sync.Map                 `json:"graphs"`
	Forecasts     []Forecasts               `json:"forecasts"`
	Decisions     []Decision                `json:"decisions"`
	Lifecycle     *sync.Map                 `json:"lifecycle"`
	Findings      []Finding                 `json:"findings"`
	Hypotheses    []Hypothesis              `json:"hypotheses"`
	Categories    map[string][]Category     `json:"categories"`
	Manifold      *sync.Map                 `json:"manifold"`
	Cognition     *sync.Map                 `json:"cognition"`
	Resonance     []any                     `json:"resonance"`
	Causal        []any                     `json:"causal"`
	cutIncomplete bool
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis() *Thesis {
	return &Thesis{
		At:           time.Now().UTC(),
		Positions:    &sync.Map{},
		Holdings:     &sync.Map{},
		Decisions:    make([]Decision, 0),
		CrossSection: NewCrossSection(),
		Graphs:       &sync.Map{},
		Forecasts:    make([]Forecasts, 0),
		Lifecycle:    &sync.Map{},
		Findings:     make([]Finding, 0),
		Hypotheses:   make([]Hypothesis, 0),
		Categories:   make(map[string][]Category),
		Measurements: make(map[string][]*Measurement),
		Manifold:     &sync.Map{},
		Cognition:    &sync.Map{},
		Resonance:    make([]any, 0),
		Causal:       make([]any, 0),
	}
}

/*
ResetTick replaces per-tick evidence while preserving Lifecycle phases that
span orders (entry/exit submitted) and Findings owned by this Thesis. Holdings
and Positions are rebuilt each cut from Admit and the desk.
*/
func (thesis *Thesis) ResetTick(at time.Time, tick int64) {
	if thesis == nil {
		return
	}

	thesis.Tick = tick
	thesis.At = at
	thesis.CrossSection = NewCrossSection()
	thesis.publish.Lock()
	thesis.Measurements = nil
	thesis.Measurements = make(map[string][]*Measurement)
	thesis.publish.Unlock()
	thesis.Forecasts = thesis.Forecasts[:0]
	thesis.Decisions = thesis.Decisions[:0]
	thesis.Hypotheses = thesis.Hypotheses[:0]
	thesis.Categories = make(map[string][]Category)
	thesis.Resonance = thesis.Resonance[:0]
	thesis.Causal = thesis.Causal[:0]
	thesis.cutIncomplete = false
	// Graphs holds resident pointers (category graph); do not clear between ticks.
	clearSyncMap(thesis.Manifold)
	clearSyncMap(thesis.Cognition)
	clearSyncMap(thesis.Positions)
	clearSyncMap(thesis.Holdings)
}

/*
StampAt sets Thesis.At to the latest measurement timestamp so Actor-driven cuts
carry market time instead of the wall clock captured at NewThesis.
*/
func (thesis *Thesis) StampAt() {
	if thesis == nil {
		return
	}

	var latest time.Time

	for _, bySymbol := range thesis.Measurements {
		for _, row := range bySymbol {
			if row == nil || row.At.IsZero() {
				continue
			}

			if row.At.After(latest) {
				latest = row.At
			}
		}
	}

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
	thesis.publish.Lock()
	defer thesis.publish.Unlock()

	for _, row := range rows {
		thesis.Measurements[row.Symbol] = append(thesis.Measurements[row.Symbol], row)
	}
}

/*
EachMeasurement walks the grouped measurement surface under the Thesis publish
read lock. Readers use it while signal actors may append rows, preventing map
iteration races without reviving the removed snapshot allocation path.
*/
func (thesis *Thesis) EachMeasurement(yield func(*Measurement) bool) {
	thesis.publish.RLock()
	defer thesis.publish.RUnlock()

	for _, rows := range thesis.Measurements {
		for _, row := range rows {
			if !yield(row) {
				return
			}
		}
	}
}

/*
clearSyncMap drops every entry from map without reallocating the header.
*/
func clearSyncMap(values *sync.Map) {
	if values == nil {
		return
	}

	values.Range(func(key, _ any) bool {
		values.Delete(key)
		return true
	})
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
MarshalJSON stores the Thesis itself while translating its concurrent maps into
ordinary JSON objects. The live Thesis remains the only state model.
*/
func (thesis *Thesis) MarshalJSON() ([]byte, error) {
	type alias Thesis
	signals := make(map[string]any)
	lifecycle := make(map[string]string)
	manifold := make(map[string]any)
	cognition := make(map[string]Cognition)
	positions := make(map[string]bool)
	holdings := make(map[string]Holding)

	thesis.Positions.Range(func(key, value any) bool {
		positions[key.(string)] = true
		return true
	})

	thesis.Holdings.Range(func(key, value any) bool {
		switch holding := value.(type) {
		case Holding:
			holdings[key.(string)] = holding
		case *Holding:
			holdings[key.(string)] = *holding
		}

		return true
	})

	thesis.Lifecycle.Range(func(key, value any) bool {
		lifecycle[key.(string)] = value.(string)
		return true
	})

	thesis.Manifold.Range(func(key, value any) bool {
		manifold[key.(string)] = value
		return true
	})

	thesis.Cognition.Range(func(key, value any) bool {
		cognition[key.(string)] = value.(Cognition)
		return true
	})

	return sonic.Marshal(struct {
		*alias
		Signals   map[string]any       `json:"signals"`
		Lifecycle map[string]string    `json:"lifecycle"`
		Manifold  map[string]any       `json:"manifold"`
		Cognition map[string]Cognition `json:"cognition"`
		Positions map[string]bool      `json:"positions"`
		Holdings  map[string]Holding   `json:"holdings"`
	}{
		alias: (*alias)(thesis), Signals: signals,
		Lifecycle: lifecycle, Manifold: manifold, Cognition: cognition,
		Positions: positions, Holdings: holdings,
	})
}

/*
UnmarshalJSON restores a persisted Thesis and rebuilds its in-process maps and
Gonum evidence graphs from their stored values.
*/
func (thesis *Thesis) UnmarshalJSON(data []byte) error {
	type alias Thesis
	decoded := struct {
		*alias
		Signals   map[string]any       `json:"signals"`
		Lifecycle map[string]string    `json:"lifecycle"`
		Manifold  map[string]any       `json:"manifold"`
		Cognition map[string]Cognition `json:"cognition"`
		Positions map[string]bool      `json:"positions"`
		Holdings  map[string]Holding   `json:"holdings"`
	}{alias: (*alias)(thesis)}

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	thesis.Graphs = &sync.Map{}
	thesis.Lifecycle = &sync.Map{}
	thesis.Manifold = &sync.Map{}
	thesis.Cognition = &sync.Map{}
	thesis.Positions = &sync.Map{}
	thesis.Holdings = &sync.Map{}

	for key, holding := range decoded.Holdings {
		seed := holding
		thesis.Holdings.Store(key, &seed)
	}

	for key := range decoded.Positions {
		thesis.Positions.Store(key, nil)
	}

	for key, value := range decoded.Lifecycle {
		thesis.Lifecycle.Store(key, value)
	}

	for key, value := range decoded.Manifold {
		thesis.Manifold.Store(key, value)
	}

	for key, value := range decoded.Cognition {
		thesis.Cognition.Store(key, value)
	}

	if thesis.CrossSection == nil {
		thesis.CrossSection = NewCrossSection()
	}

	if thesis.CrossSection.Metrics == nil {
		thesis.CrossSection.Metrics = &sync.Map{}
	}

	return nil
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
