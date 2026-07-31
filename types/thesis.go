package types

import (
	"sync"
	"time"
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

type MarketEntity string

const (
	MarketTicker MarketEntity = "ticker"
	MarketTrade  MarketEntity = "trade"
	MarketBook   MarketEntity = "book"
)

type Stamp struct {
	At     time.Time    `json:"at"`
	Entity MarketEntity `json:"entity"`
}

/*
Thesis is the durable lifecycle record from entry through post-mortem. It keeps
decisions, lifecycle, and findings; each market cut replaces per-tick evidence
in place so the object does not grow without bound.
*/
type Thesis struct {
	Status       Status                `json:"status"`
	Tick         int64                 `json:"tick"`
	At           time.Time             `json:"at"`
	CrossSection *CrossSection         `json:"crossSection"`
	Measurements *sync.Map             `json:"-"`
	Books        *sync.Map             `json:"-"`
	Graphs       *sync.Map             `json:"-"`
	Forecasts    []Forecasts           `json:"forecasts"`
	Decisions    []Decision            `json:"decisions"`
	Lifecycle    *sync.Map             `json:"lifecycle"`
	Findings     []Finding             `json:"findings"`
	Hypotheses   []Hypothesis          `json:"hypotheses"`
	Categories   map[string][]Category `json:"categories"`
	Manifold     *sync.Map             `json:"-"`
	Cognition    *sync.Map             `json:"-"`
	Resonance    *sync.Map             `json:"-"`
	Causal       *sync.Map             `json:"-"`
	Stamps       *sync.Map             `json:"-"`
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis() *Thesis {
	return &Thesis{
		Status:       INITIALIZING,
		At:           time.Now().UTC(),
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
		Stamps:       &sync.Map{},
	}
}

/*
AppendMeasuremnts appends measurements for the specified
source while safely ignoring nil input.
*/
func (thesis *Thesis) AppendMeasurements(
	source SourceType,
	measurements []*Measurement,
	stamp Stamp,
) *Thesis {
	if len(measurements) == 0 {
		return thesis
	}

	found, ok := thesis.Measurements.LoadOrStore(
		source, measurements,
	)

	if ok {
		thesis.Measurements.Store(
			source, append(
				found.([]*Measurement), measurements...,
			),
		)

		found, ok := thesis.Stamps.LoadOrStore(source, []Stamp{stamp})

		if ok {
			thesis.Stamps.Store(
				source, append(
					found.([]Stamp), stamp,
				),
			)
		}
	}

	return thesis
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
