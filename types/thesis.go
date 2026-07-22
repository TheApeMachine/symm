package types

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

const (
	ThesisKey = "thesis"

	LifecycleObserving        = "observing"
	LifecycleShaped           = "shaped"
	LifecycleEntrySelected    = "entry_selected"
	LifecycleEntrySubmitted   = "entry_submitted"
	LifecyclePartiallyEntered = "partially_entered"
	LifecycleEntered          = "entered"
	LifecycleManaging         = "managing"
	LifecycleExitSelected     = "exit_selected"
	LifecycleExitSubmitted    = "exit_submitted"
	LifecyclePartiallyExited  = "partially_exited"
	LifecycleClosed           = "closed"
	LifecyclePostMortemReady  = "postmortem_ready"
	LifecycleEvaluated        = "evaluated"
	LifecycleExpired          = "expired"
	LifecycleRejected         = "rejected"
	LifecycleInvalid          = "invalid"
)

/*
Thesis is the durable lifecycle record from entry through post-mortem. It keeps
holdings it created plus lifecycle and findings; each market cut replaces
per-tick evidence in place so the object does not grow without bound.
*/
type Thesis struct {
	checkpoint    atomic.Int64
	uiHub         chan<- []byte
	Tick          int64          `json:"tick"`
	At            time.Time      `json:"at"`
	Positions     *sync.Map      `json:"positions"`
	Holdings      *sync.Map      `json:"holdings"`
	CrossSection  *CrossSection  `json:"crossSection"`
	Measurements  []*Measurement `json:"measurements"`
	Graphs        *sync.Map      `json:"graphs"`
	Forecasts     []Forecasts    `json:"forecasts"`
	Decisions     []Decision     `json:"decisions"`
	Lifecycle     *sync.Map      `json:"lifecycle"`
	Findings      []Finding      `json:"findings"`
	Hypotheses    []Hypothesis   `json:"hypotheses"`
	Categories    []Category     `json:"categories"`
	Manifold      *sync.Map      `json:"manifold"`
	Cognition     *sync.Map      `json:"cognition"`
	Resonance     []any          `json:"resonance"`
	Causal        []any          `json:"causal"`
	cutIncomplete bool
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis(uiHub chan<- []byte) *Thesis {
	return &Thesis{
		uiHub:        uiHub,
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
		Categories:   make([]Category, 0),
		Manifold:     &sync.Map{},
		Cognition:    &sync.Map{},
		Resonance:    make([]any, 0),
		Causal:       make([]any, 0),
	}
}

/*
ResetTick replaces per-tick evidence while preserving Holdings, Lifecycle, and
Findings owned by this Thesis.
*/
func (thesis *Thesis) ResetTick(at time.Time, tick int64) {
	if thesis == nil {
		return
	}

	pending := thesis.Decisions[:0]

	for _, decision := range thesis.Decisions {
		phase, found := thesis.Lifecycle.Load(decision.Symbol)

		if decision.Action == ActionEnter && found && phase == LifecycleEntrySelected {
			pending = append(pending, decision)
		}
	}

	thesis.Tick = tick
	thesis.At = at
	thesis.CrossSection = NewCrossSection()
	thesis.Measurements = nil
	thesis.Forecasts = thesis.Forecasts[:0]
	thesis.Decisions = pending
	thesis.Hypotheses = thesis.Hypotheses[:0]
	thesis.Categories = thesis.Categories[:0]
	thesis.Resonance = thesis.Resonance[:0]
	thesis.Causal = thesis.Causal[:0]
	thesis.cutIncomplete = false
	thesis.Graphs.Clear()
	thesis.Manifold.Clear()
	thesis.Cognition.Clear()
	thesis.Positions.Clear()
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
CutSnapshot copies this tick's measurements and forecasts for history callers
that must not observe later ResetTick replacements on the durable Thesis.
*/
func (thesis *Thesis) CutSnapshot() *Thesis {
	if thesis == nil {
		return nil
	}

	snapshot := NewThesis(thesis.uiHub)
	snapshot.Tick = thesis.Tick
	snapshot.At = thesis.At
	snapshot.CrossSection = thesis.CrossSection
	snapshot.Measurements = append([]*Measurement{}, thesis.Measurements...)
	snapshot.Forecasts = append([]Forecasts{}, thesis.Forecasts...)
	snapshot.Decisions = append([]Decision{}, thesis.Decisions...)
	snapshot.Findings = append([]Finding{}, thesis.Findings...)
	snapshot.Hypotheses = append([]Hypothesis{}, thesis.Hypotheses...)
	snapshot.Categories = append([]Category{}, thesis.Categories...)

	return snapshot
}

func (thesis *Thesis) Save(dir string) error {
	target := filepath.Join(dir, ThesisKey+".json")
	json, err := thesis.MarshalJSON()

	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to create thesis directory",
			err,
		))
	}

	temporary, err := os.CreateTemp(dir, ThesisKey+"-*.tmp")

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to create thesis temp file",
			err,
		))
	}

	temporaryPath := temporary.Name()

	if _, err := temporary.Write(json); err != nil {
		temporary.Close()
		os.Remove(temporaryPath)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to write thesis checkpoint",
			err,
		))
	}

	if err := temporary.Sync(); err != nil {
		temporary.Close()
		os.Remove(temporaryPath)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to sync thesis checkpoint",
			err,
		))
	}

	if err := temporary.Close(); err != nil {
		os.Remove(temporaryPath)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to close thesis temp file",
			err,
		))
	}

	if err := os.Rename(temporaryPath, target); err != nil {
		os.Remove(temporaryPath)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to persist thesis checkpoint",
			err,
		))
	}

	return nil
}

/*
MarshalJSON stores the Thesis itself while translating its concurrent maps into
ordinary JSON objects. The live Thesis remains the only state model.
*/
func (thesis *Thesis) MarshalJSON() ([]byte, error) {
	type alias Thesis
	signals := make(map[string]any)
	graphs := make(map[string]*Graph)
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
			if holding.Stoploss != nil {
				holding.Stoploss.RLock()
				defer holding.Stoploss.RUnlock()
			}

			holdings[key.(string)] = holding
		case *Holding:
			if holding.Stoploss != nil {
				holding.Stoploss.RLock()
				defer holding.Stoploss.RUnlock()
			}

			holdings[key.(string)] = *holding
		}

		return true
	})

	thesis.Graphs.Range(func(key, value any) bool {
		graphs[key.(string)] = value.(*Graph)
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
		Graphs    map[string]*Graph    `json:"graphs"`
		Lifecycle map[string]string    `json:"lifecycle"`
		Manifold  map[string]any       `json:"manifold"`
		Cognition map[string]Cognition `json:"cognition"`
		Positions map[string]bool      `json:"positions"`
		Holdings  map[string]Holding   `json:"holdings"`
	}{
		alias: (*alias)(thesis), Signals: signals, Graphs: graphs,
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
		Graphs    map[string]*Graph    `json:"graphs"`
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

	for key, graph := range decoded.Graphs {
		thesis.Graphs.Store(key, graph)
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
