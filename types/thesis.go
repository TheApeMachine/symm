package types

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
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
Thesis is essentially the "state" of a tick. It travels across the
entire lifecycle of a tick, picking up all data along the way.
*/
type Thesis struct {
	checkpoint   atomic.Int64
	uiProjection atomic.Value
	uiHub        chan<- []byte
	Tick         int64          `json:"tick"`
	At           time.Time      `json:"at"`
	Positions    []Holding      `json:"positions"`
	CrossSection *CrossSection  `json:"crossSection"`
	Measurements []*Measurement `json:"measurements"`
	Graphs       *sync.Map      `json:"graphs"`
	Forecasts    []Forecasts    `json:"forecasts"`
	Decisions    []Decision     `json:"decisions"`
	Orders       []spot.Order   `json:"orders"`
	Lifecycle    *sync.Map      `json:"lifecycle"`
	Findings     []Finding      `json:"findings"`
	Hypotheses   []Hypothesis   `json:"hypotheses"`
	Categories   []Category     `json:"categories"`
	Manifold     *sync.Map      `json:"manifold"`
	Cognition    *sync.Map      `json:"cognition"`
	Resonance    []any          `json:"resonance"`
	Causal       []any          `json:"causal"`
}

/*
SetUIProjection records the symbol and signal source the operator is currently
inspecting so bounded analysis and websocket publication retain that view.
*/
func (thesis *Thesis) SetUIProjection(symbol string, source SourceType) {
	thesis.uiProjection.Store([2]string{symbol, string(source)})
}

/*
UIProjection returns one consistent dashboard scope while a thesis is being
published concurrently with frontend focus changes.
*/
func (thesis *Thesis) UIProjection() (string, SourceType) {
	projection := thesis.uiProjection.Load()

	if projection == nil {
		return "", ""
	}

	view := projection.([2]string)

	return view[0], SourceType(view[1])
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
	}{
		alias: (*alias)(thesis), Signals: signals, Graphs: graphs,
		Lifecycle: lifecycle, Manifold: manifold, Cognition: cognition,
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
	}{alias: (*alias)(thesis)}

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	thesis.Graphs = &sync.Map{}
	thesis.Lifecycle = &sync.Map{}
	thesis.Manifold = &sync.Map{}
	thesis.Cognition = &sync.Map{}

	for key, frame := range decoded.Graphs {
		graph := NewGraph(frame.Symbol)

		for _, node := range frame.Nodes {
			measurement := node.Measurement

			if err := graph.AddNode(&measurement); err != nil {
				return err
			}
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
NewThesis creates an empty in-process lifecycle carrier for one tick.
*/
func NewThesis(uiHub chan<- []byte) *Thesis {
	return &Thesis{
		uiHub:        uiHub,
		At:           time.Now().UTC(),
		Positions:    make([]Holding, 0),
		Decisions:    make([]Decision, 0),
		CrossSection: NewCrossSection(),
		Measurements: make([]*Measurement, 0),
		Graphs:       &sync.Map{},
		Forecasts:    make([]Forecasts, 0),
		Orders:       make([]spot.Order, 0),
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
NewSignalThesis creates the isolated subset a signal can measure concurrently.
Planner merges this evidence into the complete lifecycle Thesis after workers
return it through the result channel.
*/
func NewSignalThesis(at time.Time) *Thesis {
	return &Thesis{
		At:           at,
		CrossSection: NewCrossSection(),
		Measurements: make([]*Measurement, 0),
	}
}
