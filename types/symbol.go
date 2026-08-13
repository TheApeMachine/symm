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
	Symbol       string    `json:"symbol,omitempty"`
	Status       Status    `json:"status,omitempty"`
	Tick         int64     `json:"tick,omitempty"`
	Measurements *sync.Map `json:"-"`
	tickers      *sync.Map `json:"-"`
	trades       *sync.Map `json:"-"`
	Decisions    *sync.Map `json:"decisions,omitempty"`
	Graphs       *sync.Map `json:"graphs,omitempty"`
	Categories   *sync.Map `json:"categories,omitempty"`
	Phase        *sync.Map `json:"-"`
	Cognition    *sync.Map `json:"-"`
	Resonance    *sync.Map `json:"-"`
	Causal       *sync.Map `json:"-"`
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
		Measurements: &sync.Map{},
		Decisions:    &sync.Map{},
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
	for _, source := range TickerReceivers {
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
	for _, source := range TradeReceivers {
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

func (symbol *Symbol) AppendMeasurement(_ SourceType, measurement *Measurement) bool {
	categoryMeasurements, _ := symbol.Measurements.LoadOrStore("category", lf.NewQueue[*Measurement]())
	graphMeasurements, _ := symbol.Measurements.LoadOrStore("graph", lf.NewQueue[*Measurement]())
	manifoldMeasurements, _ := symbol.Measurements.LoadOrStore("manifold", lf.NewQueue[*Measurement]())

	categoryMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)
	graphMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)
	manifoldMeasurements.(*lf.Queue[*Measurement]).Enqueue(measurement)

	resonanceMeasurement := MeasurementToResonance(symbol.Symbol, measurement)

	if resonanceMeasurement == nil || len(resonanceMeasurement.Readings) == 0 {
		return false
	}

	resonanceMeasurements, _ := symbol.Measurements.LoadOrStore(
		"resonance",
		lf.NewQueue[*ResonanceMeasurement](),
	)
	resonanceMeasurements.(*lf.Queue[*ResonanceMeasurement]).Enqueue(resonanceMeasurement)

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
