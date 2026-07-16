package types

import (
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
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
Thesis is essentially the "state" of a tick. It travels across the
entire lifecycle of a tick, picking up all data along the way.
*/
type Thesis struct {
	checkpoint   atomic.Int64
	uiHub        chan<- []byte
	Tick         int64          `json:"tick"`
	Positions    []Holding      `json:"positions"`
	Signals      *sync.Map      `json:"signals"`
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

	thesis.Signals.Range(func(key, value any) bool {
		signals[key.(string)] = value
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

	thesis.Signals = &sync.Map{}
	thesis.Graphs = &sync.Map{}
	thesis.Lifecycle = &sync.Map{}
	thesis.Manifold = &sync.Map{}
	thesis.Cognition = &sync.Map{}

	for key, value := range decoded.Signals {
		thesis.Signals.Store(key, value)
	}
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
		Positions:    make([]Holding, 0),
		Decisions:    make([]Decision, 0),
		Signals:      &sync.Map{},
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
Publish exposes the non-empty evidence accumulated by this tick without delaying trading.
*/
func (thesis *Thesis) Publish() {
	if thesis.uiHub == nil {
		return
	}

	leader, leadershipThreshold := thesis.CrossSection.Leadership()

	thesis.Send(datura.Map[any]{
		"tick": datura.Map[any]{"count": thesis.Tick},
		"diagnostics": []datura.Map[any]{
			{
				"metrics":             thesis.CrossSection.Metrics,
				"leader":              leader,
				"leadershipThreshold": leadershipThreshold,
				"breadth":             thesis.CrossSection.Breadth(),
			},
		},
	})

	if len(thesis.Measurements) > 0 {
		thesis.Send(datura.Map[any]{
			"measurements": thesis.Measurements,
		})
	}

	if len(thesis.Decisions) > 0 {
		thesis.Send(datura.Map[any]{
			"decisions": thesis.Decisions,
		})
	}

	if len(thesis.Orders) > 0 {
		thesis.Send(datura.Map[any]{
			"orders": thesis.Orders,
		})
	}

	if len(thesis.Positions) > 0 {
		thesis.Send(datura.Map[any]{
			"positions": thesis.Positions,
		})
	}

	if thesis.Lifecycle != nil {
		lifecycle := make(map[string]string)

		thesis.Lifecycle.Range(func(key, value any) bool {
			symbol, symbolOK := key.(string)
			state, stateOK := value.(string)

			if !symbolOK || !stateOK {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"thesis contains invalid lifecycle state",
					nil,
				))

				return true
			}

			lifecycle[symbol] = state
			return true
		})

		thesis.Send(datura.Map[any]{
			"lifecycle": lifecycle,
		})
	}

	if len(thesis.Findings) > 0 {
		thesis.Send(datura.Map[any]{
			"findings": thesis.Findings,
		})
	}

	graphs := make([]GraphFrame, 0)

	if thesis.Graphs != nil {
		thesis.Graphs.Range(func(key, value any) bool {
			evidenceGraph, ok := value.(*Graph)

			if !ok {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"thesis contains an invalid evidence graph",
					nil,
				))

				return true
			}

			graphs = append(graphs, evidenceGraph.Frame())

			return true
		})
	}

	if len(graphs) > 0 {
		thesis.Send(datura.Map[any]{
			"graphs": graphs,
		})
	}

	if len(thesis.Forecasts) > 0 {
		thesis.Send(datura.Map[any]{
			"forecasts": thesis.Forecasts,
		})
	}

	if len(thesis.Hypotheses) > 0 {
		thesis.Send(datura.Map[any]{
			"hypotheses": thesis.Hypotheses,
		})
	}

	if len(thesis.Categories) > 0 {
		thesis.Send(datura.Map[any]{
			"categories": thesis.Categories,
		})
	}

	manifolds := make([]any, 0)

	thesis.Manifold.Range(func(key, value any) bool {
		manifolds = append(manifolds, value)
		return true
	})

	if len(manifolds) > 0 {
		thesis.Send(datura.Map[any]{
			"manifold": manifolds,
		})
	}

	cognition := make([]Cognition, 0)

	thesis.Cognition.Range(func(key, value any) bool {
		reading, ok := value.(Cognition)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"thesis contains invalid cognition state",
				nil,
			))

			return true
		}

		cognition = append(cognition, reading)

		return true
	})

	if len(cognition) > 0 {
		thesis.Send(datura.Map[any]{
			"cognition": cognition,
		})
	}

	if len(thesis.Resonance) > 0 {
		thesis.Send(datura.Map[any]{
			"resonance": thesis.Resonance,
		})
	}

	if len(thesis.Causal) > 0 {
		thesis.Send(datura.Map[any]{
			"causal": thesis.Causal,
		})
	}
}

func (thesis *Thesis) Send(frame datura.Map[any]) {
	select {
	case thesis.uiHub <- frame.Marshal():
	default:
	}
}
