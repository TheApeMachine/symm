package types

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/datura"
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

/*
Readiness returns the readiness of the thesis for evaluation.

A stage is ready when it has stamped the thesis, and only then. A stamp is the
one statement a solver makes about itself having run to completion, so reading
anything else — the contents of its output map, the length of a slice it
happens to fill — infers readiness from a side effect and lets a stage that
legitimately produced nothing read as never having run.
*/
type Readiness struct {
	mu          sync.RWMutex
	ui          chan []byte
	Correlation bool `json:"correlation"`
	CVD         bool `json:"cvd"`
	DepthFlow   bool `json:"depth_flow"`
	Exhaustion  bool `json:"exhaustion"`
	Hawkes      bool `json:"hawkes"`
	LeadLag     bool `json:"lead_lag"`
	Liquidity   bool `json:"liquidity"`
	PumpDump    bool `json:"pump_dump"`
	Sentiment   bool `json:"sentiment"`
	Toxicity    bool `json:"toxicity"`
	Manifold    bool `json:"manifold"`
	Resonance   bool `json:"resonance"`
	Causal      bool `json:"causal"`
	Graph       bool `json:"graph"`
	Allocation  bool `json:"allocation"`
	Decisions   bool `json:"decisions"`
	Categories  bool `json:"categories"`
	Cognition   bool `json:"cognition"`
}

func NewReadiness(ui chan []byte) Readiness {
	return Readiness{
		ui: ui,
	}
}

/*
Stamp records completion of one concurrently running signal stage.
*/
func (readiness *Readiness) Stamp(source SourceType) {
	readiness.mu.Lock()
	defer readiness.mu.Unlock()

	switch source {
	case SourceCorrelation:
		readiness.Correlation = true
	case SourceCVD:
		readiness.CVD = true
	case SourceDepthFlow:
		readiness.DepthFlow = true
	case SourceExhaustion:
		readiness.Exhaustion = true
	case SourceHawkes:
		readiness.Hawkes = true
	case SourceLeadLag:
		readiness.LeadLag = true
	case SourceLiquidity:
		readiness.Liquidity = true
	case SourcePumpDump:
		readiness.PumpDump = true
	case SourceSentiment:
		readiness.Sentiment = true
	case SourceToxicity:
		readiness.Toxicity = true
	}

	if readiness.ui != nil {
		select {
		case readiness.ui <- datura.NewMap(
			"readiness", readiness,
		).MarshalAndFree():
		default:
		}
	}
}

func (readiness *Readiness) SignalsMeasured() bool {
	readiness.mu.RLock()
	defer readiness.mu.RUnlock()

	return readiness.Correlation &&
		readiness.CVD &&
		readiness.DepthFlow &&
		readiness.Exhaustion &&
		readiness.Hawkes &&
		readiness.LeadLag &&
		readiness.Liquidity &&
		readiness.PumpDump &&
		readiness.Sentiment &&
		readiness.Toxicity
}

/*
Complete answers whether every stage that produces evidence has stamped this
tick, which is what a decision can be drawn from.

Allocation and Decisions are stamped by the decision pass rather than read by
it: orders are sized after a candidate has been judged, and a tick is only known
to have decided once it has. Requiring them here made the gate wait on the pass
it was gating, and since a tick that fails the gate is never reset, neither flag
could be raised on any later tick either.
*/
func (readiness *Readiness) Complete() bool {
	readiness.mu.RLock()
	defer readiness.mu.RUnlock()

	return readiness.SignalsMeasured() &&
		readiness.Manifold &&
		readiness.Resonance &&
		readiness.Causal &&
		readiness.Graph &&
		readiness.Categories &&
		readiness.Cognition
}

/*
Snapshot copies the stage stamps for audit and wire payloads without exposing
the mutex used by concurrently running signals.
*/
func (readiness *Readiness) Snapshot() *Readiness {
	readiness.mu.RLock()
	defer readiness.mu.RUnlock()

	return readiness.snapshot()
}

func (readiness *Readiness) snapshot() *Readiness {
	return &Readiness{
		Correlation: readiness.Correlation,
		CVD:         readiness.CVD,
		DepthFlow:   readiness.DepthFlow,
		Exhaustion:  readiness.Exhaustion,
		Hawkes:      readiness.Hawkes,
		LeadLag:     readiness.LeadLag,
		Liquidity:   readiness.Liquidity,
		PumpDump:    readiness.PumpDump,
		Sentiment:   readiness.Sentiment,
		Toxicity:    readiness.Toxicity,
		Manifold:    readiness.Manifold,
		Resonance:   readiness.Resonance,
		Causal:      readiness.Causal,
		Graph:       readiness.Graph,
		Allocation:  readiness.Allocation,
		Decisions:   readiness.Decisions,
		Categories:  readiness.Categories,
		Cognition:   readiness.Cognition,
	}
}

/*
Reset clears every stage stamp for the next evaluation epoch.
*/
func (readiness *Readiness) Reset() {
	readiness.mu.Lock()
	defer readiness.mu.Unlock()

	readiness.Correlation = false
	readiness.CVD = false
	readiness.DepthFlow = false
	readiness.Exhaustion = false
	readiness.Hawkes = false
	readiness.LeadLag = false
	readiness.Liquidity = false
	readiness.PumpDump = false
	readiness.Sentiment = false
	readiness.Toxicity = false
	readiness.Manifold = false
	readiness.Resonance = false
	readiness.Causal = false
	readiness.Graph = false
	readiness.Allocation = false
	readiness.Decisions = false
	readiness.Categories = false
	readiness.Cognition = false
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

/*
Thesis owns canonical evidence across every evaluated epoch that contributes to
one decision. It closes only after the planner emits the completed decision set;
broker execution and settlement continue in their own lifecycle.
*/
type Thesis struct {
	Readiness
	marketMu      sync.RWMutex
	measurementMu sync.RWMutex
	Status        Status                `json:"status"`
	Tick          int64                 `json:"tick"`
	At            time.Time             `json:"at"`
	CrossSection  *CrossSection         `json:"crossSection"`
	Measurements  *sync.Map             `json:"-"`
	Tickers       *sync.Map             `json:"-"`
	Trades        *sync.Map             `json:"-"`
	Graphs        *sync.Map             `json:"-"`
	Decisions     []Decision            `json:"decisions"`
	Lifecycle     *sync.Map             `json:"lifecycle"`
	Findings      []Finding             `json:"findings"`
	Hypotheses    []Hypothesis          `json:"hypotheses"`
	Categories    map[string][]Category `json:"categories"`
	Manifold      *sync.Map             `json:"-"`
	Cognition     *sync.Map             `json:"-"`
	Resonance     *sync.Map             `json:"-"`
	Causal        *sync.Map             `json:"-"`
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis(ui chan []byte) *Thesis {
	return &Thesis{
		Status:       READY,
		At:           time.Now().UTC(),
		Decisions:    make([]Decision, 0),
		CrossSection: NewCrossSection(),
		Graphs:       &sync.Map{},
		Lifecycle:    &sync.Map{},
		Findings:     make([]Finding, 0),
		Hypotheses:   make([]Hypothesis, 0),
		Categories:   make(map[string][]Category),
		Measurements: &sync.Map{},
		Tickers:      &sync.Map{},
		Trades:       &sync.Map{},
		Manifold:     &sync.Map{},
		Cognition:    &sync.Map{},
		Resonance:    &sync.Map{},
		Causal:       &sync.Map{},
		Readiness:    NewReadiness(ui),
	}
}

/*
PrepareNextEvaluation clears only the working state whose meaning is limited to
one evaluated market epoch. Canonical market observations, measurements, and
lifecycle evidence remain available for the complete decision cycle.
*/
func (thesis *Thesis) PrepareNextEvaluation() *Thesis {
	if thesis == nil {
		return nil
	}

	thesis.At = time.Now().UTC()
	thesis.CrossSection = NewCrossSection()
	thesis.Graphs.Clear()
	thesis.Manifold.Clear()
	thesis.Cognition.Clear()
	thesis.Resonance.Clear()
	thesis.Causal.Clear()
	thesis.Readiness.Reset()
	thesis.Decisions = make([]Decision, 0)
	thesis.Categories = make(map[string][]Category)

	return thesis
}

/*
CloseCycle clears retained evidence after the planner has emitted the completed
decision set. Decision emission is the terminal boundary; order settlement is
a separate broker lifecycle and does not extend this evidence cycle.
*/
func (thesis *Thesis) CloseCycle() *Thesis {
	if thesis == nil {
		return nil
	}

	thesis.PrepareNextEvaluation()

	thesis.marketMu.Lock()
	thesis.Tickers.Clear()
	thesis.Trades.Clear()
	thesis.marketMu.Unlock()

	thesis.measurementMu.Lock()
	thesis.Measurements.Clear()
	thesis.measurementMu.Unlock()

	thesis.Lifecycle.Clear()
	thesis.Findings = make([]Finding, 0)
	thesis.Hypotheses = make([]Hypothesis, 0)

	return thesis
}

/*
Series returns this tick's measurements for one symbol in observation order,
oldest first. Signals store their rows by source, but a reader asking how a
symbol is developing needs one timeline across every source that observed it:
the last row answers what is true now, while the shape of the series answers
where it is heading.
*/
func (thesis *Thesis) Series(symbol string) []*Measurement {
	series := make([]*Measurement, 0)

	if thesis == nil || thesis.Measurements == nil {
		return series
	}

	thesis.Measurements.Range(func(_, value any) bool {
		rows, ok := value.([]*Measurement)

		if !ok {
			return true
		}

		for _, row := range rows {
			if row == nil || row.Symbol != symbol {
				continue
			}

			series = append(series, row)
		}

		return true
	})

	slices.SortStableFunc(series, func(left, right *Measurement) int {
		return cmp.Compare(left.At.UnixNano(), right.At.UnixNano())
	})

	return series
}

/*
Symbols returns every symbol this tick carries measurements for.
*/
func (thesis *Thesis) Symbols() []string {
	symbols := make([]string, 0)

	if thesis == nil || thesis.Measurements == nil {
		return symbols
	}

	seen := make(map[string]struct{})

	thesis.Measurements.Range(func(_, value any) bool {
		rows, ok := value.([]*Measurement)

		if !ok {
			return true
		}

		for _, row := range rows {
			if row == nil || row.Symbol == "" {
				continue
			}

			if _, ok := seen[row.Symbol]; ok {
				continue
			}

			seen[row.Symbol] = struct{}{}
			symbols = append(symbols, row.Symbol)
		}

		return true
	})

	slices.Sort(symbols)

	return symbols
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
