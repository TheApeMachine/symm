package types

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Symbol is an alternative grouping of measurements, and is used in logic that
legitimately needs to fan out measurements by symbol, rather than by source.
It should be kept extremely simple, and lean, and is not an invitation to
start adding additional complexity beyond what is truly earned.
*/
type Symbol struct {
	ID           SymbolID                                   `json:"id,omitempty"`
	Symbol       string                                     `json:"symbol,omitempty"`
	Status       Status                                     `json:"status,omitempty"`
	Tick         int64                                      `json:"tick,omitempty"`
	Measurements *transport.MapReduce[*types.Measurement]   `json:"-"`
	tickers      *transport.MapReduce[kraken.TickerData]    `json:"-"`
	trades       *transport.MapReduce[kraken.TradeData]     `json:"-"`
	level3       *transport.MapReduce[kraken.Level3Data]    `json:"-"`
	executions   *transport.MapReduce[kraken.ExecutionData] `json:"-"`
	Decisions    *transport.MapReduce[Decision]             `json:"decisions,omitempty"`
	Positions    *transport.MapReduce[any]                  `json:"positions,omitempty"`
	Graphs       *transport.MapReduce[*Graph]               `json:"graphs,omitempty"`
	Categories   *transport.MapReduce[[]Category]           `json:"categories,omitempty"`
	Phase        *transport.MapReduce[PhaseReading]         `json:"-"`
	Cognition    *transport.MapReduce[Cognition]            `json:"-"`
	Resonance    *transport.MapReduce[any]                  `json:"-"`
	Causal       *transport.MapReduce[map[string]any]       `json:"-"`
}

/*
NewSymbol creates empty measurement state for one market symbol.
*/
func NewSymbol(name string) *Symbol {
	symbol := &Symbol{
		Symbol: name,
		Status: READY,
		tickers: transport.NewMapReduce[kraken.TickerData](
			append(
				TickerReceiverStrings,
				string(SourceResonance),
				string(SourceDesk),
			), nil, nil,
		),
		trades: transport.NewMapReduce[kraken.TradeData](
			TradeReceiverStrings, nil, nil,
		),
		level3: transport.NewMapReduce[kraken.Level3Data](
			Level3ReceiverStrings, nil, nil,
		),
		executions: transport.NewMapReduce[kraken.ExecutionData](
			[]string{string(SourceDesk)}, nil, nil,
		),
		Measurements: transport.NewMapReduce[*types.Measurement](
			LogicSourceStrings, nil, nil,
		),
		Decisions: transport.NewMapReduce[Decision](
			[]string{}, nil, nil,
		),
		Positions: transport.NewMapReduce[any](
			[]string{}, nil, nil,
		),
		Graphs: transport.NewMapReduce[*Graph](
			[]string{string(SourcePlanner), string(SourceGraph)}, nil, nil,
		),
		Categories: transport.NewMapReduce[[]Category](
			[]string{string(SourceGraph), string(SourceCognition)}, nil, nil,
		),
		Phase: transport.NewMapReduce[PhaseReading](
			[]string{string(SourceGraph)}, nil, nil,
		),
		Cognition: transport.NewMapReduce[Cognition](
			[]string{string(SourceGraph)}, nil, nil,
		),
		Resonance: transport.NewMapReduce[any](
			[]string{string(SourceCausal), string(SourceGraph)}, nil, nil,
		),
		Causal: transport.NewMapReduce[map[string]any](
			[]string{string(SourceGraph), string(SourceCausal)}, nil, nil,
		),
	}

	symbol.Phase.Register("audit")
	symbol.Cognition.Register("audit")
	symbol.Resonance.Register("audit")
	symbol.Causal.Register("audit")
	symbol.Categories.Register("audit")
	symbol.Decisions.Register("audit")
	symbol.Graphs.Register("audit")
	symbol.Measurements.Register("audit")

	return symbol
}

/*
AppendTicker routes a ticker only to the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendTicker(ticker kraken.TickerData) {
	symbol.tickers.Push(ticker)
}

/*
HasTickers reports whether this symbol has an unconsumed ticker queued for the
signal owners, letting a self-run consumer probe for work before draining.
*/
func (symbol *Symbol) HasTickers() bool {
	return symbol.tickers.Length() > 0
}

/*
HasTrades reports whether this symbol has an unconsumed trade queued for the
signal owners.
*/
func (symbol *Symbol) HasTrades() bool {
	return symbol.trades.Length() > 0
}

/*
HasLevel3 reports whether this symbol has an unconsumed level-3 frame queued
for the signal owners.
*/
func (symbol *Symbol) HasLevel3() bool {
	return symbol.level3.Length() > 0
}

func (symbol *Symbol) HasExecutions() bool {
	return symbol.executions.Length() > 0
}

/*
AppendTrade routes a trade only to the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendTrade(trade kraken.TradeData) {
	symbol.trades.Push(trade)
}

/*
AppendLevel3 retains one accepted order-identity frame for the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendLevel3(level3 kraken.Level3Data) {
	symbol.level3.Push(level3)
}

func (symbol *Symbol) AppendExecution(execution kraken.ExecutionData) {
	symbol.executions.Push(execution)
}

/*
AppendMeasurement pushes one measured measurement — stamping the owning
symbol's tick at the boundary — and routes it to every solver cursor that
consumes it. Signals project their numeric Frame output into the nomagique
Measurement shape via AddMetrics and push that shape here.
*/
func (symbol *Symbol) AppendMeasurement(measurement *types.Measurement) error {
	symbol.Measurements.Push(measurement)
	return nil
}

/*
CheckpointState materializes the drained decision artifacts needed to audit a
completed thesis lifecycle. Each MapReduce column is drained on a dedicated
audit cursor so the checkpoint does not collide with live solver consumption.
*/
func (symbol *Symbol) CheckpointState() any {
	checkpoint := struct {
		ID         SymbolID `json:"id,omitempty"`
		Symbol     string   `json:"symbol"`
		Status     Status   `json:"status"`
		Tick       int64    `json:"tick"`
		Decisions  any      `json:"decisions"`
		Graphs     any      `json:"graphs"`
		Categories any      `json:"categories"`
		Phase      any      `json:"phase"`
		Cognition  any      `json:"cognition"`
		Resonance  any      `json:"resonance"`
		Causal     any      `json:"causal"`
	}{
		ID:         symbol.ID,
		Symbol:     symbol.Symbol,
		Status:     symbol.Status,
		Tick:       symbol.Tick,
		Decisions:  drained(symbol.Decisions),
		Graphs:     drained(symbol.Graphs),
		Categories: drainedCategories(symbol),
		Phase:      drained(symbol.Phase),
		Cognition:  drained(symbol.Cognition),
		Resonance:  drained(symbol.Resonance),
		Causal:     drained(symbol.Causal),
	}

	return checkpoint
}

func drained[T any](mr *transport.MapReduce[T]) []T {
	rows := make([]T, 0)

	sink := func(item T) bool { return true }
	for item := range mr.Drain("audit", sink) {
		rows = append(rows, item)
	}

	return rows
}

func drainedCategories(symbol *Symbol) []Category {
	rows := make([]Category, 0)

	for batch := range symbol.Categories.Drain("audit", func(_ []Category) bool {
		return true
	}) {
		rows = append(rows, batch...)
	}

	return rows
}

/*
QueueDepths reports the current pending item count of every per-symbol stage
buffer. The keys are stable wire names the diagnostics page renders as labeled
lanes; a nil queue is reported with a zero entry rather than omitted so the
consumer can render a full row even before the producer first writes.
*/
func (symbol *Symbol) QueueDepths() map[string]uint64 {
	out := make(map[string]uint64, 12)

	if symbol == nil {
		return out
	}

	if symbol.tickers != nil {
		out["tickers"] = symbol.tickers.Length()
	}
	if symbol.trades != nil {
		out["trades"] = symbol.trades.Length()
	}
	if symbol.level3 != nil {
		out["level3"] = symbol.level3.Length()
	}
	if symbol.Measurements != nil {
		out["measurements"] = symbol.Measurements.Length()
	}
	if symbol.Decisions != nil {
		out["decisions"] = symbol.Decisions.Length()
	}
	if symbol.Positions != nil {
		out["positions"] = symbol.Positions.Length()
	}
	if symbol.Graphs != nil {
		out["graphs"] = symbol.Graphs.Length()
	}
	if symbol.Categories != nil {
		out["categories"] = symbol.Categories.Length()
	}
	if symbol.Phase != nil {
		out["phase"] = symbol.Phase.Length()
	}
	if symbol.Cognition != nil {
		out["cognition"] = symbol.Cognition.Length()
	}
	if symbol.Resonance != nil {
		out["resonance"] = symbol.Resonance.Length()
	}
	if symbol.Causal != nil {
		out["causal"] = symbol.Causal.Length()
	}

	return out
}

/*
MarketTickers drains this source's ticker queue up to an event-time cut taken
when the drain starts. Ingress can outpace the reader, so a drain that chased
queue emptiness would never end under sustained load; rows stamped after the
cut are processed one last time and then left for the next pass.
*/
func (symbol *Symbol) MarketTickers(source SourceType) iter.Seq[kraken.TickerData] {
	cut := time.Now().UTC()

	return symbol.tickers.Drain(string(source), func(ticker kraken.TickerData) bool {
		if ticker.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

/*
MarketTrades drains this source's trade queue up to an event-time cut taken
when the drain starts, on the same terms as MarketTickers.
*/
func (symbol *Symbol) MarketTrades(source SourceType) iter.Seq[kraken.TradeData] {
	cut := time.Now().UTC()

	return symbol.trades.Drain(string(source), func(trade kraken.TradeData) bool {
		if trade.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

/*
MarketLevel3 drains this source's accepted order-identity frames in transport
order, up to an event-time cut taken when the drain starts, on the same terms
as MarketTickers.
*/
func (symbol *Symbol) MarketLevel3(source SourceType) iter.Seq[kraken.Level3Data] {
	cut := time.Now().UTC()

	return symbol.level3.Drain(string(source), func(level3 kraken.Level3Data) bool {
		if level3.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

func (symbol *Symbol) MarketMeasurements(solver string) iter.Seq[*types.Measurement] {
	cut := time.Now().UTC()

	return symbol.Measurements.Drain(solver, func(measurement *types.Measurement) bool {
		if measurement == nil {
			return true
		}

		return measurement.At.Before(cut) || measurement.At.Equal(cut)
	})
}

/*
MarketPhase drains this symbol's universe phase sweep to the consuming stage.
*/
func (symbol *Symbol) MarketPhase(stage SourceType) iter.Seq[PhaseReading] {
	return symbol.Phase.Drain(string(stage), func(_ PhaseReading) bool {
		return true
	})
}

/*
MarketCognition drains this symbol's cognition readings to the consuming stage.
*/
func (symbol *Symbol) MarketCognition(stage SourceType) iter.Seq[Cognition] {
	return symbol.Cognition.Drain(string(stage), func(_ Cognition) bool {
		return true
	})
}

/*
MarketCategories drains this symbol's category batches to the consuming stage.
*/
func (symbol *Symbol) MarketCategories(stage SourceType) iter.Seq[[]Category] {
	return symbol.Categories.Drain(string(stage), func(_ []Category) bool {
		return true
	})
}

/*
MarketResonance drains this symbol's resonance artifacts to the consuming stage.
*/
func (symbol *Symbol) MarketResonance(stage SourceType) iter.Seq[any] {
	return symbol.Resonance.Drain(string(stage), func(_ any) bool {
		return true
	})
}

/*
MarketCausal drains this symbol's causal artifacts to the consuming stage.
*/
func (symbol *Symbol) MarketCausal(stage SourceType) iter.Seq[map[string]any] {
	return symbol.Causal.Drain(string(stage), func(_ map[string]any) bool {
		return true
	})
}

/*
MarketGraphs drains this symbol's lifecycle graphs to the consuming stage.
*/
func (symbol *Symbol) MarketGraphs(stage SourceType) iter.Seq[*Graph] {
	return symbol.Graphs.Drain(string(stage), func(_ *Graph) bool {
		return true
	})
}
