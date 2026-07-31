package types

import (
	"sync"
	"time"
)

var thesisSignalSources = []SourceType{
	SourceCorrelation,
	SourceCVD,
	SourceDepthFlow,
	SourceExhaustion,
	SourceHawkes,
	SourceLeadLag,
	SourceLiquidity,
	SourcePumpDump,
	SourceSentiment,
	SourceToxicity,
}

var thesisSignalSourceSet = map[SourceType]struct{}{
	SourceCorrelation: {},
	SourceCVD:         {},
	SourceDepthFlow:   {},
	SourceExhaustion:  {},
	SourceHawkes:      {},
	SourceLeadLag:     {},
	SourceLiquidity:   {},
	SourcePumpDump:    {},
	SourceSentiment:   {},
	SourceToxicity:    {},
}

type Readiness struct {
	Signals    bool `json:"signals"`
	Manifold   bool `json:"manifold"`
	Resonance  bool `json:"resonance"`
	Causal     bool `json:"causal"`
	Graph      bool `json:"graph"`
	Allocation bool `json:"allocation"`
	Decisions  bool `json:"decisions"`
}

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
	Source SourceType   `json:"source"`
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
Readiness returns the readiness of the thesis for evaluation.
*/
func (thesis *Thesis) Readiness() Readiness {
	readiness := Readiness{}
	counts := thesis.stampCounts()

	readiness.Signals = counts.signals == len(thesisSignalSources)
	readiness.Manifold = readiness.Signals && counts.manifold > 0
	readiness.Resonance = readiness.Manifold && counts.resonance > 0
	readiness.Causal = readiness.Resonance && counts.causal > 0
	readiness.Graph = readiness.Causal && thesis.hasEntries(thesis.Graphs)
	readiness.Allocation = readiness.Graph && len(thesis.Forecasts) > 0
	readiness.Decisions = readiness.Allocation && len(thesis.Decisions) > 0

	return readiness
}

type thesisStampCounts struct {
	signals   int
	manifold  int
	resonance int
	causal    int
}

func (thesis *Thesis) stampCounts() thesisStampCounts {
	counts := thesisStampCounts{}
	seenSignals := make(map[SourceType]struct{})

	if thesis == nil || thesis.Stamps == nil {
		return counts
	}

	thesis.Stamps.Range(func(_, value any) bool {
		stamps, ok := value.([]Stamp)

		if !ok || len(stamps) == 0 {
			return true
		}

		for _, stamp := range stamps {
			if _, ok := thesisSignalSourceSet[stamp.Source]; ok {
				if _, seen := seenSignals[stamp.Source]; !seen {
					seenSignals[stamp.Source] = struct{}{}
					counts.signals++
				}

				continue
			}

			switch stamp.Source {
			case SourceCategory:
				counts.manifold++
			case SourceResonance:
				counts.resonance++
			case SourceCausal:
				counts.causal++
			}
		}

		return true
	})

	return counts
}

func (thesis *Thesis) hasEntries(values *sync.Map) bool {
	if thesis == nil || values == nil {
		return false
	}

	hasEntries := false

	values.Range(func(_, _ any) bool {
		hasEntries = true

		return false
	})

	return hasEntries
}

/*
Reset clears transient per-cycle evidence after decisions are produced while
keeping the durable lifecycle record alive for the next thesis cycle.
*/
func (thesis *Thesis) Reset() *Thesis {
	if thesis == nil {
		return nil
	}

	thesis.At = time.Now().UTC()
	thesis.Tick = 0
	thesis.CrossSection = NewCrossSection()
	thesis.Measurements = &sync.Map{}
	thesis.Books = &sync.Map{}
	thesis.Graphs = &sync.Map{}
	thesis.Forecasts = make([]Forecasts, 0)
	thesis.Decisions = make([]Decision, 0)
	thesis.Findings = make([]Finding, 0)
	thesis.Hypotheses = make([]Hypothesis, 0)
	thesis.Categories = make(map[string][]Category)
	thesis.Manifold = &sync.Map{}
	thesis.Cognition = &sync.Map{}
	thesis.Resonance = &sync.Map{}
	thesis.Causal = &sync.Map{}
	thesis.Stamps = &sync.Map{}

	return thesis
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

		return thesis
	}

	thesis.Stamps.Store(source, []Stamp{stamp})

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
