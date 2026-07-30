package types

import (
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
func NewThesis(bookManager *spot.BookManager) *Thesis {
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
		BookManager:  bookManager,
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
	thesis.cutIncomplete = false
	thesis.checkpoint.Store(0)

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

func (thesis *Thesis) AppendMeasuremnts(
	source SourceType, measurements []*Measurement,
) *Thesis {
	if measurements == nil {
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
	}

	return thesis
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
NoteLifecycle records a symbol phase transition. Desk inventory keeps its own
Holding.Status; this map is the strategy phase, not a second journal.
*/
func (thesis *Thesis) NoteLifecycle(symbol, status string, _ time.Time) {
	if thesis == nil || symbol == "" || status == "" {
		return
	}

	thesis.Lifecycle.Store(symbol, status)
}
