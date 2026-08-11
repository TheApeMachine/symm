package types

import (
	"fmt"
	"iter"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"golang.design/x/lockfree/lf"
)

var signals = []SourceType{
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
Symbol is an alternative grouping of measurements, and is used in logic that
legitimately needs to fan out measurements by symbol, rather than by source.
It should be kept extremely simple, and lean, and is not an invitation to
start adding additional complexity beyond what is truly earned. Symbols
carry their own readiness state, which is important for the resonance solver,
which needs a stable measurement set to train on.
*/
type Symbol struct {
	Symbol       string    `json:"symbol,omitempty"`
	Status       Status    `json:"status,omitempty"`
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

	for _, source := range signals {
		symbol.tickers.Store(source, lf.NewQueue[kraken.TickerData]())
		symbol.trades.Store(source, lf.NewQueue[kraken.TradeData]())
		symbol.Measurements.Store(source, lf.NewQueue[*Measurement]())
	}

	return symbol
}

func (symbol *Symbol) AppendTicker(ticker kraken.TickerData) {
	for _, source := range signals {
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
	for _, source := range signals {
		value, ok := symbol.trades.Load(source)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				fmt.Sprintf("ticker cursor not found for source %s", source),
				nil,
			))

			continue
		}

		value.(*lf.Queue[kraken.TradeData]).Enqueue(trade)
	}
}

func (symbol *Symbol) AppendMeasurement(source SourceType, measurement *Measurement) {
	value, ok := symbol.Measurements.Load(source)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("measurement queue not found for source %s", source),
			nil,
		))

		return
	}

	value.(*lf.Queue[*Measurement]).Enqueue(measurement)
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
				yield(data)
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
				yield(data)
			}
		}
	}
}

func (symbol *Symbol) MarketMeasurements(measurement *Measurement) iter.Seq[*Measurement] {
	measurements, ok := symbol.Measurements.Load(measurement.Source)

	if !ok {
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
				yield(data)
			}
		}
	}
}

/*
MarshalJSON materializes every concurrent symbol state map for durable thesis
checkpoints.
*/
func (symbol *Symbol) MarshalJSON() ([]byte, error) {
	return datura.NewMap(symbol).MarshalAndFree(), nil
}
