package types

import (
	"fmt"
	"iter"
	"sync"

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
	ID           SymbolID                     `json:"id,omitempty"`
	Symbol       string                       `json:"symbol,omitempty"`
	Status       Status                       `json:"status,omitempty"`
	Tick         int64                        `json:"tick,omitempty"`
	Measurements *sync.Map                    `json:"-"`
	Latest       *sync.Map                    `json:"-"`
	tickers      *sync.Map                    `json:"-"`
	trades       *sync.Map                    `json:"-"`
	level3       *lf.Queue[kraken.Level3Data] `json:"-"`
	Decisions    *sync.Map                    `json:"decisions,omitempty"`
	Positions    *sync.Map                    `json:"positions,omitempty"`
	Graphs       *sync.Map                    `json:"graphs,omitempty"`
	Categories   *sync.Map                    `json:"categories,omitempty"`
	Phase        *sync.Map                    `json:"-"`
	Cognition    *sync.Map                    `json:"-"`
	Resonance    *sync.Map                    `json:"-"`
	Causal       *sync.Map                    `json:"-"`
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
		level3:       lf.NewQueue[kraken.Level3Data](),
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

	return symbol
}

func (symbol *Symbol) AppendTicker(ticker kraken.TickerData) {
	symbol.AppendTickerTo(ticker, TickerReceivers)
}

/*
AppendTickerTo routes a ticker only to the signal owners selected by the
streaming topology.
*/
func (symbol *Symbol) AppendTickerTo(
	ticker kraken.TickerData,
	receivers []SourceType,
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
	}
}

func (symbol *Symbol) AppendTrade(trade kraken.TradeData) {
	symbol.AppendTradeTo(trade, TradeReceivers)
}

/*
AppendTradeTo routes a trade only to the signal owners selected by the
streaming topology.
*/
func (symbol *Symbol) AppendTradeTo(
	trade kraken.TradeData,
	receivers []SourceType,
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
	}
}

/*
AppendLevel3 retains one accepted order-identity frame for its symbol owner.
*/
func (symbol *Symbol) AppendLevel3(level3 kraken.Level3Data) {
	symbol.level3.Enqueue(level3)
}

func (symbol *Symbol) AppendMeasurement(_ SourceType, measurement *Measurement) bool {
	symbol.Latest.Store(measurement.Key(), measurement)
	categoryMeasurements, _ := symbol.Measurements.LoadOrStore("category", lf.NewQueue[*Measurement]())
	graphMeasurements, _ := symbol.Measurements.LoadOrStore("graph", lf.NewQueue[*Measurement]())
	manifoldMeasurements, _ := symbol.Measurements.LoadOrStore("manifold", lf.NewQueue[*Measurement]())

	categoryMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)
	graphMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)
	manifoldMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)

	return symbol.AppendResonanceMeasurement(
		MeasurementToResonance(symbol.Symbol, measurement),
	)
}

/*
AppendResonanceMeasurement queues one ordered predictor observation. A market
mark without new signal readings is still ground truth for forecasts issued by
the existing model.
*/
func (symbol *Symbol) AppendResonanceMeasurement(
	measurement *ResonanceMeasurement,
) bool {
	if measurement == nil ||
		(measurement.Mark <= 0 && len(measurement.Readings) == 0) {
		return false
	}

	resonanceMeasurements, _ := symbol.Measurements.LoadOrStore(
		"resonance",
		lf.NewQueue[*ResonanceMeasurement](),
	)
	resonanceMeasurements.(*lf.Queue[*ResonanceMeasurement]).Enqueue(measurement)

	return true
}

func (symbol *Symbol) MarketTickers(source SourceType) iter.Seq[kraken.TickerData] {
	ticker, ok := symbol.tickers.Load(source)

	if !ok {
		return func(yield func(kraken.TickerData) bool) {}
	}

	return func(yield func(kraken.TickerData) bool) {
		var (
			data kraken.TickerData
			ok   bool = true
		)

		for ok {
			data, ok = ticker.(*lf.Queue[kraken.TickerData]).Dequeue()

			if ok {
				if !yield(data) {
					return
				}
			}
		}
	}
}

func (symbol *Symbol) MarketTrades(source SourceType) iter.Seq[kraken.TradeData] {
	trade, ok := symbol.trades.Load(source)

	if !ok {
		return func(yield func(kraken.TradeData) bool) {}
	}

	return func(yield func(kraken.TradeData) bool) {
		var (
			data kraken.TradeData
			ok   bool = true
		)

		for ok {
			data, ok = trade.(*lf.Queue[kraken.TradeData]).Dequeue()

			if ok {
				if !yield(data) {
					return
				}
			}
		}
	}
}

/*
MarketLevel3 drains accepted order-identity frames in transport order.
*/
func (symbol *Symbol) MarketLevel3() iter.Seq[kraken.Level3Data] {
	return func(yield func(kraken.Level3Data) bool) {
		for {
			data, ok := symbol.level3.Dequeue()

			if !ok {
				return
			}

			if !yield(data) {
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
		var (
			data *Measurement
			ok   bool = true
		)

		for ok {
			data, ok = measurements.(*lf.Queue[*Measurement]).Dequeue()

			if ok {
				if !yield(data) {
					return
				}
			}
		}
	}
}

func (symbol *Symbol) ResonanceMeasurements() iter.Seq[*ResonanceMeasurement] {
	measurements, found := symbol.Measurements.Load("resonance")

	if !found {
		return func(yield func(*ResonanceMeasurement) bool) {}
	}

	return func(yield func(*ResonanceMeasurement) bool) {
		var (
			data *ResonanceMeasurement
			ok   bool = true
		)
		for ok {
			data, ok = measurements.(*lf.Queue[*ResonanceMeasurement]).Dequeue()

			if ok {
				if !yield(data) {
					return
				}
			}
		}
	}
}
