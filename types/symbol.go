package types

import (
	"iter"
	"sync"
	"sync/atomic"
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
	ID           SymbolID                                 `json:"id,omitempty"`
	Symbol       string                                   `json:"symbol,omitempty"`
	Status       Status                                   `json:"status,omitempty"`
	Tick         int64                                    `json:"tick,omitempty"`
	Measurements *transport.MapReduce[*types.Measurement] `json:"-"`
	Latest       *sync.Map                                `json:"-"`
	tickers      *transport.MapReduce[kraken.TickerData]  `json:"-"`
	trades       *transport.MapReduce[kraken.TradeData]   `json:"-"`
	level3       *transport.MapReduce[kraken.Level3Data]  `json:"-"`
	pending      atomic.Int64                             `json:"-"`
	bookRevision atomic.Uint64                            `json:"-"`
	bookAt       atomic.Int64                             `json:"-"`
	Decisions    *sync.Map                                `json:"decisions,omitempty"`
	Positions    *sync.Map                                `json:"positions,omitempty"`
	Graphs       *sync.Map                                `json:"graphs,omitempty"`
	Categories   *sync.Map                                `json:"categories,omitempty"`
	Phase        *sync.Map                                `json:"-"`
	Cognition    *sync.Map                                `json:"-"`
	Resonance    *sync.Map                                `json:"-"`
	Causal       *sync.Map                                `json:"-"`
}

/*
CheckpointState materializes the concurrent decision artifacts needed to audit
an admitted entry without serializing streaming queues or solver cursors.
*/
func (symbol *Symbol) CheckpointState() any {
	checkpoint := struct {
		ID         SymbolID       `json:"id,omitempty"`
		Symbol     string         `json:"symbol"`
		Status     Status         `json:"status"`
		Tick       int64          `json:"tick"`
		Decisions  map[string]any `json:"decisions"`
		Graphs     map[string]any `json:"graphs"`
		Categories map[string]any `json:"categories"`
		Phase      map[string]any `json:"phase"`
		Cognition  map[string]any `json:"cognition"`
		Resonance  map[string]any `json:"resonance"`
		Causal     map[string]any `json:"causal"`
	}{
		ID:        symbol.ID,
		Symbol:    symbol.Symbol,
		Status:    symbol.Status,
		Tick:      symbol.Tick,
		Decisions: checkpointMap(symbol.Decisions, nil),
		Graphs: checkpointMap(symbol.Graphs, func(value any) any {
			graph, valid := value.(interface{ CheckpointState() any })

			if !valid {
				return value
			}

			return graph.CheckpointState()
		}),
		Categories: checkpointMap(symbol.Categories, nil),
		Phase:      checkpointMap(symbol.Phase, nil),
		Cognition:  checkpointMap(symbol.Cognition, nil),
		Resonance:  checkpointMap(symbol.Resonance, nil),
		Causal:     checkpointMap(symbol.Causal, nil),
	}

	return checkpoint
}

func checkpointMap(
	source *sync.Map,
	transform func(any) any,
) map[string]any {
	checkpoint := make(map[string]any)

	if source == nil {
		return checkpoint
	}

	source.Range(func(key, value any) bool {
		name, valid := key.(string)

		if !valid || name == "" {
			return true
		}

		if transform != nil {
			value = transform(value)
		}

		checkpoint[name] = value
		return true
	})

	return checkpoint
}

/*
NewSymbol creates empty measurement state for one market symbol.
*/
func NewSymbol(name string, ui chan []byte) *Symbol {
	symbol := &Symbol{
		Symbol: name,
		Status: READY,
		tickers: transport.NewMapReduce[kraken.TickerData](
			TickerReceiverStrings, nil, nil,
		),
		trades: transport.NewMapReduce[kraken.TradeData](
			TradeReceiverStrings, nil, nil,
		),
		level3: transport.NewMapReduce[kraken.Level3Data](
			Level3ReceiverStrings, nil, nil,
		),
		Measurements: transport.NewMapReduce[*types.Measurement](
			LogicSourceStrings, nil, nil,
		),
		Latest:     &sync.Map{},
		Decisions:  &sync.Map{},
		Positions:  &sync.Map{},
		Graphs:     &sync.Map{},
		Categories: &sync.Map{},
		Phase:      &sync.Map{},
		Cognition:  &sync.Map{},
		Resonance:  &sync.Map{},
		Causal:     &sync.Map{},
	}

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

	if observedAt := level3ObservedAt(level3); !observedAt.IsZero() {
		nanos := observedAt.UnixNano()

		for {
			current := symbol.bookAt.Load()

			if nanos <= current || symbol.bookAt.CompareAndSwap(current, nanos) {
				break
			}
		}
	}

	symbol.bookRevision.Add(1)
}

/*
BookRevision returns the monotone accepted-frame revision and its event-time
high-water mark. A revision belongs to the complete authoritative book state,
not to whichever individual levels happen to survive in that state.
*/
func (symbol *Symbol) BookRevision() (uint64, time.Time) {
	if symbol == nil {
		return 0, time.Time{}
	}

	nanos := symbol.bookAt.Load()

	if nanos == 0 {
		return symbol.bookRevision.Load(), time.Time{}
	}

	return symbol.bookRevision.Load(), time.Unix(0, nanos).UTC()
}

func level3ObservedAt(level3 kraken.Level3Data) time.Time {
	observedAt := level3.Timestamp

	for _, orders := range [][]kraken.Level3Order{level3.Bids, level3.Asks} {
		for _, order := range orders {
			if order.Timestamp.After(observedAt) {
				observedAt = order.Timestamp
			}
		}
	}

	return observedAt
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
Pending reports whether any market queue still holds undrained rows. Appends
count rows in, the drain seqs count them out, so the check is one load.
*/
func (symbol *Symbol) Pending() bool {
	return symbol.pending.Load() > 0
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
		if observedAt := level3ObservedAt(level3); !observedAt.IsZero() && observedAt.After(cut) {
			return false
		}

		return true
	})
}

func (symbol *Symbol) MarketMeasurements(solver string) iter.Seq[*types.Measurement] {
	cut := time.Now().UTC().Unix()

	return symbol.Measurements.Drain(solver, func(measurement *types.Measurement) bool {
		return measurement.At <= cut
	})
}
