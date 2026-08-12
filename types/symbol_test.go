package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestSymbolNewSymbol(t *testing.T) {
	Convey("Given a symbol name", t, func() {
		symbol := NewSymbol("BTC/USD", nil)

		Convey("It should initialize empty symbol stream state", func() {
			So(symbol.Symbol, ShouldEqual, "BTC/USD")
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldNotBeNil)
		})
	})
}

func TestSymbolAppendMeasurement(t *testing.T) {
	Convey("Given one measurement appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		measurement := &Measurement{ID: "first", Source: SourceHawkes, Symbol: "BTC/USD"}

		symbol.AppendMeasurement(SourceHawkes, measurement)

		Convey("It should expose the row to every downstream measurement iterator", func() {
			for _, solver := range []string{"category", "graph", "manifold"} {
				rows := make([]*Measurement, 0)

				for row := range symbol.MarketMeasurements(solver) {
					rows = append(rows, row)
				}

				So(rows, ShouldResemble, []*Measurement{measurement})
			}

			_, found := symbol.Measurements.Load("resonance")
			So(found, ShouldBeFalse)
		})
	})
}

func TestSymbolResonanceInputs(t *testing.T) {
	Convey("Given varied complete cuts followed by an incomplete cut", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		value := 0.0

		for epoch := range 2 {
			value = float64(epoch)

			for _, source := range SignalSources {
				symbol.AppendMeasurement(source, &Measurement{
					Source: source,
					Symbol: "BTC/USD",
					Metrics: map[string]MetricSample{
						"score": {Normalized: &value},
					},
				})
			}
		}

		value = 2

		for _, source := range SignalSources[:len(SignalSources)-1] {
			symbol.AppendMeasurement(source, &Measurement{
				Source: source,
				Symbol: "BTC/USD",
				Tick:   3,
				Metadata: map[string]float64{
					"last_price": 100,
				},
				Metrics: map[string]MetricSample{
					"score": {Normalized: &value},
				},
			})
		}

		Convey("It should enqueue one model-ready vector only after the last source", func() {
			lastSource := SignalSources[len(SignalSources)-1]
			ready := symbol.AppendMeasurement(lastSource, &Measurement{
				Source: lastSource,
				Symbol: "BTC/USD",
				Tick:   3,
				Metadata: map[string]float64{
					"last_price": 100,
				},
				Metrics: map[string]MetricSample{
					"score": {Normalized: &value},
				},
			})

			So(ready, ShouldBeTrue)
		})
	})
}

func TestSymbolAppendTicker(t *testing.T) {
	Convey("Given one ticker appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		ticker := kraken.TickerData{Symbol: "BTC/USD"}

		symbol.AppendTicker(ticker)

		Convey("It should queue the ticker only for ticker receivers", func() {
			for _, source := range TickerReceivers {
				rows := make([]kraken.TickerData, 0)

				for row := range symbol.MarketTickers(source) {
					rows = append(rows, row)
				}

				So(rows, ShouldResemble, []kraken.TickerData{ticker})
			}

			_, found := symbol.tickers.Load(SourcePumpDump)
			So(found, ShouldBeTrue)
			_, found = symbol.tickers.Load(SourceCVD)
			So(found, ShouldBeFalse)
			_, found = symbol.tickers.Load(SourceToxicity)
			So(found, ShouldBeFalse)
		})
	})
}

func TestSymbolAppendTrade(t *testing.T) {
	Convey("Given one trade appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		trade := kraken.TradeData{Symbol: "BTC/USD"}

		symbol.AppendTrade(trade)

		Convey("It should queue the trade only for trade receivers", func() {
			for _, source := range TradeReceivers {
				rows := make([]kraken.TradeData, 0)

				for row := range symbol.MarketTrades(source) {
					rows = append(rows, row)
				}

				So(rows, ShouldResemble, []kraken.TradeData{trade})
			}

			_, found := symbol.trades.Load(SourceCVD)
			So(found, ShouldBeTrue)
			_, found = symbol.trades.Load(SourceToxicity)
			So(found, ShouldBeTrue)
			_, found = symbol.trades.Load(SourceCorrelation)
			So(found, ShouldBeFalse)
			_, found = symbol.trades.Load(SourceSentiment)
			So(found, ShouldBeFalse)
		})
	})
}

func BenchmarkSymbolAppendMeasurement(b *testing.B) {
	measurement := &Measurement{
		ID: "hawkes", Source: SourceHawkes, Symbol: "BTC/USD",
	}
	b.ReportAllocs()

	for b.Loop() {
		symbol := NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurement(SourceHawkes, measurement)
	}
}

func BenchmarkSymbolResonanceInputs(b *testing.B) {
	symbol := NewSymbol("BTC/USD", nil)
	value := 0.0

	for epoch := range 2 {
		value = float64(epoch)

		for _, source := range SignalSources {
			symbol.AppendMeasurement(source, &Measurement{
				Source: source,
				Symbol: "BTC/USD",
				Metrics: map[string]MetricSample{
					"score": {Normalized: &value},
				},
			})
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		value++

		for _, source := range SignalSources {
			symbol.AppendMeasurement(source, &Measurement{
				Source: source,
				Symbol: "BTC/USD",
				Metrics: map[string]MetricSample{
					"score": {Normalized: &value},
				},
			})
		}

		for _, consumer := range []string{"category", "graph", "manifold"} {
			for range symbol.MarketMeasurements(consumer) {
			}
		}
	}
}

func BenchmarkSymbolAppendTicker(b *testing.B) {
	symbol := NewSymbol("BTC/USD", nil)
	ticker := kraken.TickerData{Symbol: "BTC/USD"}
	b.ReportAllocs()

	for b.Loop() {
		symbol.AppendTicker(ticker)

		for _, source := range TickerReceivers {
			for range symbol.MarketTickers(source) {
			}
		}
	}
}

func BenchmarkSymbolAppendTrade(b *testing.B) {
	symbol := NewSymbol("BTC/USD", nil)
	trade := kraken.TradeData{Symbol: "BTC/USD"}
	b.ReportAllocs()

	for b.Loop() {
		symbol.AppendTrade(trade)

		for _, source := range TradeReceivers {
			for range symbol.MarketTrades(source) {
			}
		}
	}
}
