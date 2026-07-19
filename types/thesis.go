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
	checkpoint   atomic.Int64
	uiHub        chan<- []byte
	marketFrame  *MarketFrame
	Tick         int64          `json:"tick"`
	At           time.Time      `json:"at"`
	Positions    *sync.Map      `json:"positions"`
	Holdings     *sync.Map      `json:"holdings"`
	CrossSection *CrossSection  `json:"crossSection"`
	Measurements []*Measurement `json:"measurements"`
	Graphs       *sync.Map      `json:"graphs"`
	Forecasts    []Forecasts    `json:"forecasts"`
	Decisions    []Decision     `json:"decisions"`
	Lifecycle    *sync.Map      `json:"lifecycle"`
	Findings     []Finding      `json:"findings"`
	Hypotheses   []Hypothesis   `json:"hypotheses"`
	Categories   []Category     `json:"categories"`
	Manifold     *sync.Map      `json:"manifold"`
	Cognition    *sync.Map      `json:"cognition"`
	Resonance    []any          `json:"resonance"`
	Causal       []any          `json:"causal"`
	// cutIncomplete is set when an interested signal worker skipped this cut;
	// Decide then manages open lots only and refuses fresh enters.
	cutIncomplete bool
}

/*
NewThesis creates a Thesis with empty durable maps and no cut evidence yet.
*/
func NewThesis(uiHub chan<- []byte, marketFrame *MarketFrame) *Thesis {
	return &Thesis{
		uiHub:        uiHub,
		At:           time.Now().UTC(),
		marketFrame:  marketFrame,
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
ResetCut replaces per-tick evidence for a new market cut while preserving
Holdings, Lifecycle, and Findings owned by this Thesis.
*/
func (thesis *Thesis) ResetCut(frame *MarketFrame, tick int64) {
	if thesis == nil {
		return
	}

	thesis.marketFrame = frame
	thesis.Tick = tick
	thesis.At = time.Time{}

	if frame != nil {
		thesis.At = frame.At
		thesis.CrossSection = frame.CrossSection
	}

	thesis.Measurements = nil
	thesis.Forecasts = thesis.Forecasts[:0]
	thesis.Decisions = thesis.Decisions[:0]
	thesis.Hypotheses = thesis.Hypotheses[:0]
	thesis.Categories = thesis.Categories[:0]
	thesis.Resonance = thesis.Resonance[:0]
	thesis.Causal = thesis.Causal[:0]
	thesis.cutIncomplete = false
	clearSyncMap(thesis.Graphs)
	clearSyncMap(thesis.Manifold)
	clearSyncMap(thesis.Cognition)
	clearSyncMap(thesis.Positions)
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
CutSnapshot copies this cut's measurements and forecasts for history callers
that must not observe later ResetCut replacements on the durable Thesis.
*/
func (thesis *Thesis) CutSnapshot() *Thesis {
	if thesis == nil {
		return nil
	}

	snapshot := NewThesis(thesis.uiHub, thesis.marketFrame)
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
	graphs := make(map[string]GraphFrame)
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

	thesis.Graphs.Range(func(key, value any) bool {
		graphs[key.(string)] = value.(*Graph).Frame()
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
		Signals   map[string]any        `json:"signals"`
		Graphs    map[string]GraphFrame `json:"graphs"`
		Lifecycle map[string]string     `json:"lifecycle"`
		Manifold  map[string]any        `json:"manifold"`
		Cognition map[string]Cognition  `json:"cognition"`
		Positions map[string]bool       `json:"positions"`
		Holdings  map[string]Holding    `json:"holdings"`
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
		Signals   map[string]any        `json:"signals"`
		Graphs    map[string]GraphFrame `json:"graphs"`
		Lifecycle map[string]string     `json:"lifecycle"`
		Manifold  map[string]any        `json:"manifold"`
		Cognition map[string]Cognition  `json:"cognition"`
		Positions map[string]bool       `json:"positions"`
		Holdings  map[string]Holding    `json:"holdings"`
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

	for key, frame := range decoded.Graphs {
		graph := NewGraph(frame.Symbol)

		for _, node := range frame.Nodes {
			graph.RestoreNode(node.Key, node.Kind, node.Category, node.Measurement)
		}

		for _, edge := range frame.Edges {
			graph.Relate(edge.From, edge.To, edge.Type, edge.At, edge.ObservedFrom)
		}

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

	thesis.CrossSection.index = make(map[string]int, len(thesis.CrossSection.Metrics))

	for index, metric := range thesis.CrossSection.Metrics {
		thesis.CrossSection.index[metric.Symbol] = index
	}

	return nil
}

/*
Market returns the central immutable market cut for this thesis.
*/
func (thesis *Thesis) Market() *MarketFrame {
	return thesis.marketFrame
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
