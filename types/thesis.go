package types

import (
	"sync"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

const (
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
	Tick         int64
	Positions    []Holding
	Signals      *sync.Map
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
	leader, leadershipThreshold := thesis.CrossSection.Leadership()

	frame := datura.Map[any]{
		"tick": datura.Map[any]{"count": thesis.Tick},
		"diagnostics": []datura.Map[any]{
			{
				"metrics":             thesis.CrossSection.Metrics,
				"leader":              leader,
				"leadershipThreshold": leadershipThreshold,
				"breadth":             thesis.CrossSection.Breadth(),
			},
		},
	}

	if len(thesis.Measurements) > 0 {
		frame["measurements"] = thesis.Measurements
	}

	if len(thesis.Decisions) > 0 {
		frame["decisions"] = thesis.Decisions
	}

	if len(thesis.Orders) > 0 {
		frame["orders"] = thesis.Orders
	}

	if len(thesis.Positions) > 0 {
		frame["positions"] = thesis.Positions
	}

	if thesis.Lifecycle != nil {
		frame["lifecycle"] = thesis.Lifecycle
	}

	if len(thesis.Findings) > 0 {
		frame["findings"] = thesis.Findings
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
		frame["graphs"] = graphs
	}

	if len(thesis.Forecasts) > 0 {
		frame["forecasts"] = thesis.Forecasts
	}

	if len(thesis.Hypotheses) > 0 {
		frame["hypotheses"] = thesis.Hypotheses
	}

	if len(thesis.Categories) > 0 {
		frame["categories"] = thesis.Categories
	}

	manifolds := make([]any, 0)

	thesis.Manifold.Range(func(key, value any) bool {
		manifolds = append(manifolds, value)
		return true
	})

	if len(manifolds) > 0 {
		frame["manifold"] = manifolds
	}

	cognition := make([]Cognition, 0)

	thesis.Cognition.Range(func(key, value any) bool {
		reading, ok := value.(Cognition)

		if ok {
			cognition = append(cognition, reading)
		}

		return true
	})

	if len(cognition) > 0 {
		frame["cognition"] = cognition
	}

	if len(thesis.Resonance) > 0 {
		frame["resonance"] = thesis.Resonance
	}

	if len(thesis.Causal) > 0 {
		frame["causal"] = thesis.Causal
	}

	select {
	case thesis.uiHub <- frame.Marshal():
	default:
	}
}
