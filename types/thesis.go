package types

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
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
	publish       *sync.RWMutex
	Tick          int64                 `json:"tick"`
	At            time.Time             `json:"at"`
	Positions     *sync.Map             `json:"positions"`
	Holdings      *sync.Map             `json:"holdings"`
	CrossSection  *CrossSection         `json:"crossSection"`
	Measurements  *sync.Map             `json:"measurements"`
	Graphs        *sync.Map             `json:"graphs"`
	Forecasts     []Forecasts           `json:"forecasts"`
	Decisions     []Decision            `json:"decisions"`
	Lifecycle     *sync.Map             `json:"lifecycle"`
	Findings      []Finding             `json:"findings"`
	Hypotheses    []Hypothesis          `json:"hypotheses"`
	Categories    map[string][]Category `json:"categories"`
	Manifold      *sync.Map             `json:"manifold"`
	Cognition     *sync.Map             `json:"cognition"`
	Resonance     *sync.Map             `json:"resonance"`
	Causal        *sync.Map             `json:"causal"`
	cutIncomplete bool
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis() *Thesis {
	return &Thesis{
		At:           time.Now().UTC(),
		publish:      &sync.RWMutex{},
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
		Measurements: &sync.Map{},
		Manifold:     &sync.Map{},
		Cognition:    &sync.Map{},
		Resonance:    &sync.Map{},
		Causal:       &sync.Map{},
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
	thesis.cutIncomplete = false
	// Graphs holds resident pointers (category graph); do not clear between ticks.
	thesis.Manifold.Clear()
	thesis.Cognition.Clear()
	thesis.Positions.Clear()
	thesis.Holdings.Clear()
}

/*
StampAt sets Thesis.At to the latest measurement timestamp so Actor-driven cuts
carry market time instead of the wall clock captured at NewThesis.
*/
func (thesis *Thesis) StampAt() {
	if thesis == nil {
		return
	}

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
MarshalJSON stores the Thesis itself while translating its concurrent maps into
ordinary JSON objects. The live Thesis remains the only state model.
*/
func (thesis *Thesis) MarshalJSON() ([]byte, error) {
	mapped := datura.NewMap()
	defer mapped.Free()
	mapped["tick"] = thesis.Tick
	mapped["at"] = thesis.At
	mapped["forecasts"] = thesis.Forecasts
	mapped["decisions"] = thesis.Decisions
	mapped["findings"] = thesis.Findings
	mapped["hypotheses"] = thesis.Hypotheses
	mapped["categories"] = thesis.Categories

	mappedPositions := datura.NewMap()
	defer mappedPositions.Free()

	mapped["positions"] = mappedPositions

	thesis.Positions.Range(func(key, value any) bool {
		mapped["positions"].(datura.Map[any])[key.(string)] = true
		return true
	})

	mappedHoldings := datura.NewMap()
	defer mappedHoldings.Free()

	mapped["holdings"] = mappedHoldings

	thesis.Holdings.Range(func(key, value any) bool {
		switch holding := value.(type) {
		case Holding:
			mapped["holdings"].(datura.Map[any])[key.(string)] = holding
		case *Holding:
			mapped["holdings"].(datura.Map[any])[key.(string)] = *holding
		}

		return true
	})

	mappedLifecycle := datura.NewMap()
	defer mappedLifecycle.Free()

	mapped["lifecycle"] = mappedLifecycle

	thesis.Lifecycle.Range(func(key, value any) bool {
		mapped["lifecycle"].(datura.Map[any])[key.(string)] = value.(string)
		return true
	})

	mappedManifold := datura.NewMap()
	defer mappedManifold.Free()

	mapped["manifold"] = mappedManifold

	thesis.Manifold.Range(func(key, value any) bool {
		mapped["manifold"].(datura.Map[any])[key.(string)] = value
		return true
	})

	mappedCognition := datura.NewMap()
	defer mappedCognition.Free()

	mapped["cognition"] = mappedCognition

	thesis.Cognition.Range(func(key, value any) bool {
		mapped["cognition"].(datura.Map[any])[key.(string)] = value
		return true
	})

	mappedResonance := datura.NewMap()
	defer mappedResonance.Free()

	mapped["resonance"] = mappedResonance

	thesis.Resonance.Range(func(key, value any) bool {
		mapped["resonance"].(datura.Map[any])[key.(string)] = value
		return true
	})

	mappedCausal := datura.NewMap()
	defer mappedCausal.Free()

	mapped["causal"] = mappedCausal

	thesis.Causal.Range(func(key, value any) bool {
		mapped["causal"].(datura.Map[any])[key.(string)] = value
		return true
	})

	mappedMeasurements := datura.NewMap()
	defer mappedMeasurements.Free()

	mapped["measurements"] = mappedMeasurements

	thesis.Measurements.Range(func(key, value any) bool {
		switch measurement := value.(type) {
		case Measurement:
			mapped["measurements"].(datura.Map[any])[key.(string)] = measurement
		case *Measurement:
			mapped["measurements"].(datura.Map[any])[key.(string)] = *measurement
		}

		return true
	})

	mappedGraphs := datura.NewMap()
	defer mappedGraphs.Free()

	mapped["graphs"] = mappedGraphs

	thesis.Graphs.Range(func(key, value any) bool {
		mapped["graphs"].(datura.Map[any])[key.(string)] = value
		return true
	})

	return mapped.Marshal(), nil
}

/*
UnmarshalJSON restores a persisted Thesis and rebuilds its in-process maps and
Gonum evidence graphs from their stored values.
*/
func (thesis *Thesis) UnmarshalJSON(data []byte) error {
	type alias Thesis
	decoded := struct {
		*alias
		Graphs    map[string]any       `json:"graphs"`
		Signals   map[string]any       `json:"signals"`
		Lifecycle map[string]string    `json:"lifecycle"`
		Manifold  map[string]any       `json:"manifold"`
		Cognition map[string]Cognition `json:"cognition"`
		Resonance map[string]any       `json:"resonance"`
		Causal    map[string]any       `json:"causal"`
		Positions map[string]bool      `json:"positions"`
		Holdings  map[string]Holding   `json:"holdings"`
	}{alias: (*alias)(thesis)}

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	thesis.Graphs = &sync.Map{}
	thesis.publish = &sync.RWMutex{}
	thesis.Lifecycle = &sync.Map{}
	thesis.Manifold = &sync.Map{}
	thesis.Cognition = &sync.Map{}
	thesis.Resonance = &sync.Map{}
	thesis.Causal = &sync.Map{}
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

	for key, value := range decoded.Resonance {
		thesis.Resonance.Store(key, value)
	}

	for key, value := range decoded.Causal {
		thesis.Causal.Store(key, value)
	}

	for key, value := range decoded.Graphs {
		thesis.Graphs.Store(key, value)
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
