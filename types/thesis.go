package types

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
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

	/*
		MarketDerived marks a stamp left by a stage that reasons over other
		stages' output rather than observing the market directly.
	*/
	MarketDerived MarketEntity = "derived"
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
	Tickers      *sync.Map             `json:"-"`
	Trades       *sync.Map             `json:"-"`
	Books        *sync.Map             `json:"-"`
	Graphs       *sync.Map             `json:"-"`
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

A stage is ready when it has stamped the thesis, and only then. A stamp is the
one statement a solver makes about itself having run to completion, so reading
anything else — the contents of its output map, the length of a slice it
happens to fill — infers readiness from a side effect and lets a stage that
legitimately produced nothing read as never having run.
*/
func (thesis *Thesis) Readiness() Readiness {
	readiness := Readiness{}
	counts := thesis.stampCounts()

	readiness.Signals = counts.signals > 0
	readiness.Manifold = readiness.Signals && counts.manifold > 0
	readiness.Resonance = readiness.Manifold && counts.resonance > 0
	readiness.Causal = readiness.Resonance && counts.causal > 0
	readiness.Graph = readiness.Causal && counts.graph > 0

	/*
		Allocation is the planner's own stage rather than a solver's, so it
		carries no stamp of its own; a thesis is allocatable once the evidence
		behind it is complete.
	*/
	readiness.Allocation = readiness.Graph
	readiness.Decisions = readiness.Allocation && len(thesis.Decisions) > 0

	return readiness
}

/*
thesisStampCounts tracks counts for the signals, manifold, resonance, causal,
and graph stages separately.
*/
type thesisStampCounts struct {
	signals   int
	manifold  int
	resonance int
	causal    int
	graph     int
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
			case SourceManifold:
				counts.manifold++
			case SourceResonance:
				counts.resonance++
			case SourceCausal:
				counts.causal++
			case SourceGraph:
				counts.graph++
			}
		}

		return true
	})

	return counts
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

	/*
		Tick is the monotonic count of evaluated ticks and deliberately
		survives a reset; zeroing it would restart the sequence every time
		the evidence is cleared.
	*/
	thesis.CrossSection = NewCrossSection()
	thesis.Measurements = &sync.Map{}
	thesis.Books = &sync.Map{}
	thesis.Graphs = &sync.Map{}
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

func (thesis *Thesis) Market() (
	[]kraken.TickerData,
	[]kraken.TradeData,
	*sync.Map,
) {
	return thesis.MarketTickers(), thesis.MarketTrades(), thesis.Books
}

func (thesis *Thesis) MarketTickers() []kraken.TickerData {
	tickers := make([]kraken.TickerData, 0)

	thesis.Tickers.Range(func(_, value any) bool {
		if ticker, ok := value.(kraken.TickerData); ok {
			tickers = append(tickers, ticker)
		}

		return true
	})

	slices.SortFunc(tickers, func(left, right kraken.TickerData) int {
		return cmp.Compare(left.Timestamp.UnixNano(), right.Timestamp.UnixNano())
	})

	return tickers
}

func (thesis *Thesis) MarketTrades() []kraken.TradeData {
	trades := make([]kraken.TradeData, 0)

	thesis.Trades.Range(func(_, value any) bool {
		if trade, ok := value.(kraken.TradeData); ok {
			trades = append(trades, trade)
		}

		return true
	})

	slices.SortStableFunc(trades, func(left, right kraken.TradeData) int {
		return cmp.Compare(left.Timestamp.UnixNano(), right.Timestamp.UnixNano())
	})

	return trades
}

/*
StampSource records that one component ran to completion on this thesis. A
stage stamps only once it has actually contributed, so downstream components
can tell whether their inputs exist rather than inferring it from whatever
happens to be in the maps.
*/
func (thesis *Thesis) StampSource(source SourceType, entity MarketEntity) *Thesis {
	if thesis == nil || thesis.Stamps == nil {
		return thesis
	}

	stamp := Stamp{
		At:     time.Now().UTC(),
		Entity: entity,
		Source: source,
	}

	found, loaded := thesis.Stamps.LoadOrStore(source, []Stamp{stamp})

	if loaded {
		thesis.Stamps.Store(source, append(found.([]Stamp), stamp))
	}

	return thesis
}

/*
Stamped reports whether every named source has run on this thesis, which is
how a component decides it has everything it needs to run itself.
*/
func (thesis *Thesis) Stamped(sources ...SourceType) bool {
	if thesis == nil || thesis.Stamps == nil {
		return false
	}

	for _, source := range sources {
		found, ok := thesis.Stamps.Load(source)

		if !ok {
			return false
		}

		stamps, ok := found.([]Stamp)

		if !ok || len(stamps) == 0 {
			return false
		}
	}

	return true
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

	stamp.Source = source

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
