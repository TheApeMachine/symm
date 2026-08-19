package types

import (
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"golang.design/x/lockfree/lf"
)

/*
Symbol is an alternative grouping of measurements, and is used in logic that
legitimately needs to fan out measurements by symbol, rather than by source.
It should be kept extremely simple, and lean, and is not an invitation to
start adding additional complexity beyond what is truly earned.
*/
type Symbol struct {
	ID           SymbolID      `json:"id,omitempty"`
	Symbol       string        `json:"symbol,omitempty"`
	Status       Status        `json:"status,omitempty"`
	Tick         int64         `json:"tick,omitempty"`
	Measurements *sync.Map     `json:"-"`
	Latest       *sync.Map     `json:"-"`
	tickers      *sync.Map     `json:"-"`
	trades       *sync.Map     `json:"-"`
	level3       *sync.Map     `json:"-"`
	pending      atomic.Int64  `json:"-"`
	bookRevision atomic.Uint64 `json:"-"`
	bookAt       atomic.Int64  `json:"-"`
	Decisions    *sync.Map     `json:"decisions,omitempty"`
	Positions    *sync.Map     `json:"positions,omitempty"`
	Graphs       *sync.Map     `json:"graphs,omitempty"`
	Categories   *sync.Map     `json:"categories,omitempty"`
	Phase        *sync.Map     `json:"-"`
	Cognition    *sync.Map     `json:"-"`
	Resonance    *sync.Map     `json:"-"`
	Causal       *sync.Map     `json:"-"`
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
		Symbol:       name,
		Status:       READY,
		tickers:      &sync.Map{},
		trades:       &sync.Map{},
		level3:       &sync.Map{},
		Measurements: &sync.Map{},
		Latest:       &sync.Map{},
		Decisions:    &sync.Map{},
		Positions:    &sync.Map{},
		Graphs:       &sync.Map{},
		Categories:   &sync.Map{},
		Phase:        &sync.Map{},
		Cognition:    &sync.Map{},
		Resonance:    &sync.Map{},
		Causal:       &sync.Map{},
	}

	for _, source := range TickerReceivers {
		symbol.tickers.Store(source, lf.NewQueue[kraken.TickerData]())
	}

	for _, source := range TradeReceivers {
		symbol.trades.Store(source, lf.NewQueue[kraken.TradeData]())
	}

	for _, source := range Level3Receivers {
		symbol.level3.Store(source, lf.NewQueue[kraken.Level3Data]())
	}

	return symbol
}

/*
AppendTicker routes a ticker only to the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendTicker(
	ticker kraken.TickerData, receivers []SourceType,
) {
	for _, source := range receivers {
		value, ok := symbol.tickers.Load(source)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				fmt.Sprintf("ticker cursor not found for source %s", source),
				nil,
			))

			continue
		}

		value.(*lf.Queue[kraken.TickerData]).Enqueue(ticker)
		symbol.pending.Add(1)
	}
}

/*
AppendTrade routes a trade only to the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendTrade(
	trade kraken.TradeData, receivers []SourceType,
) {
	for _, source := range receivers {
		value, ok := symbol.trades.Load(source)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				fmt.Sprintf("trade cursor not found for source %s", source),
				nil,
			))

			continue
		}

		value.(*lf.Queue[kraken.TradeData]).Enqueue(trade)
		symbol.pending.Add(1)
	}
}

/*
AppendLevel3 retains one accepted order-identity frame for the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendLevel3(level3 kraken.Level3Data, receivers []SourceType) {
	for _, source := range receivers {
		value, ok := symbol.level3.Load(source)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				fmt.Sprintf("level3 cursor not found for source %s", source),
				nil,
			))

			continue
		}

		value.(*lf.Queue[kraken.Level3Data]).Enqueue(level3)
		symbol.pending.Add(1)
	}

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
AppendMeasurement routes one measurement to every solver cursor that consumes
it, retains the latest row per source, and mirrors it into the resonance
predictor queue. Measurements stream in one at a time; no batch is formed.
*/
func (symbol *Symbol) AppendMeasurement(measurement *Measurement) {
	if measurement == nil {
		return
	}

	categoryMeasurements, _ := symbol.Measurements.LoadOrStore("category", lf.NewQueue[*Measurement]())
	graphMeasurements, _ := symbol.Measurements.LoadOrStore("graph", lf.NewQueue[*Measurement]())
	resonanceMeasurements, _ := symbol.Measurements.LoadOrStore("resonance", lf.NewQueue[*Measurement]())

	if measurement.Source == SourceHawkes {
		manifoldMeasurements, _ := symbol.Measurements.LoadOrStore("manifold", lf.NewQueue[*Measurement]())
		manifoldMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)
	}

	categoryMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)
	graphMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)
	resonanceMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)
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
	ticker, ok := symbol.tickers.Load(source)

	if !ok {
		return func(yield func(kraken.TickerData) bool) {}
	}

	return func(yield func(kraken.TickerData) bool) {
		cut := time.Now().UTC()

		var (
			data kraken.TickerData
			open bool = true
		)

		for open {
			data, open = ticker.(*lf.Queue[kraken.TickerData]).Dequeue()

			if !open {
				return
			}

			symbol.pending.Add(-1)

			if !yield(data) {
				return
			}

			if data.Timestamp.After(cut) {
				return
			}
		}
	}
}

/*
MarketTrades drains this source's trade queue up to an event-time cut taken
when the drain starts, on the same terms as MarketTickers.
*/
func (symbol *Symbol) MarketTrades(source SourceType) iter.Seq[kraken.TradeData] {
	trade, ok := symbol.trades.Load(source)

	if !ok {
		return func(yield func(kraken.TradeData) bool) {}
	}

	return func(yield func(kraken.TradeData) bool) {
		cut := time.Now().UTC()

		var (
			data kraken.TradeData
			open bool = true
		)

		for open {
			data, open = trade.(*lf.Queue[kraken.TradeData]).Dequeue()

			if !open {
				return
			}

			symbol.pending.Add(-1)

			if !yield(data) {
				return
			}

			if data.Timestamp.After(cut) {
				return
			}
		}
	}
}

/*
MarketLevel3 drains this source's accepted order-identity frames in transport
order, up to an event-time cut taken when the drain starts, on the same terms
as MarketTickers.
*/
func (symbol *Symbol) MarketLevel3(source SourceType) iter.Seq[kraken.Level3Data] {
	queue, ok := symbol.level3.Load(source)

	if !ok {
		return func(yield func(kraken.Level3Data) bool) {}
	}

	return func(yield func(kraken.Level3Data) bool) {
		cut := time.Now().UTC()

		for {
			data, ok := queue.(*lf.Queue[kraken.Level3Data]).Dequeue()

			if !ok {
				return
			}

			symbol.pending.Add(-1)

			if !yield(data) {
				return
			}

			if data.Timestamp.After(cut) {
				return
			}
		}
	}
}

func (symbol *Symbol) MarketMeasurements(solver string) iter.Seq[*Measurement] {
	measurements, found := symbol.Measurements.Load(solver)

	if !found {
		return func(yield func(*Measurement) bool) {}
	}

	return func(yield func(*Measurement) bool) {
		for {
			data, ok := measurements.(*lf.Queue[*Measurement]).Dequeue()

			if !ok {
				return
			}

			if !yield(data) {
				return
			}
		}
	}
}
