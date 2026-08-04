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

/*
Readiness returns the readiness of the thesis for evaluation.

A stage is ready when it has stamped the thesis, and only then. A stamp is the
one statement a solver makes about itself having run to completion, so reading
anything else — the contents of its output map, the length of a slice it
happens to fill — infers readiness from a side effect and lets a stage that
legitimately produced nothing read as never having run.
*/
type Readiness struct {
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

func NewReadiness() Readiness {
	return Readiness{
		Correlation: false,
		CVD:         false,
		DepthFlow:   false,
		Exhaustion:  false,
		Hawkes:      false,
		LeadLag:     false,
		Liquidity:   false,
		PumpDump:    false,
		Sentiment:   false,
		Toxicity:    false,
		Manifold:    false,
		Resonance:   false,
		Causal:      false,
		Graph:       false,
		Allocation:  false,
		Decisions:   false,
		Categories:  false,
		Cognition:    false,
	}
}

func (readiness *Readiness) SignalsMeasured() bool {
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
	return readiness.SignalsMeasured() &&
		readiness.Manifold &&
		readiness.Resonance &&
		readiness.Causal &&
		readiness.Graph &&
		readiness.Categories &&
		readiness.Cognition
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
Thesis is the durable lifecycle record from entry through post-mortem. It keeps
decisions, lifecycle, and findings; each market cut replaces per-tick evidence
in place so the object does not grow without bound.
*/
type Thesis struct {
	Readiness
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
		Readiness:    NewReadiness(),
	}
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

	// Tick is the monotonic count of evaluated ticks and deliberately
	// survives a reset; zeroing it would restart the sequence every time
	// the evidence is cleared.
	thesis.CrossSection = NewCrossSection()
	thesis.Measurements.Clear()
	thesis.Books.Clear()
	thesis.Graphs.Clear()
	thesis.Manifold.Clear()
	thesis.Cognition.Clear()
	thesis.Resonance.Clear()
	thesis.Causal.Clear()
	thesis.Readiness = NewReadiness()
	thesis.Decisions = make([]Decision, 0)
	thesis.Findings = make([]Finding, 0)
	thesis.Hypotheses = make([]Hypothesis, 0)
	thesis.Categories = make(map[string][]Category)

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
