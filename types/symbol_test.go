package types

import (
	"testing"
	"time"

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
			So(symbol.Positions, ShouldNotBeNil)
		})
	})
}

func TestSymbolAppendMeasurement(t *testing.T) {
	Convey("Given one raw-only measurement appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		measurement := &Measurement{ID: "first", Source: SourceHawkes, Symbol: "BTC/USD"}

		symbol.AppendMeasurement(measurement)

		Convey("It should expose the row without waking predictive coding", func() {
			for _, solver := range []string{"category", "graph"} {
				rows := make([]*Measurement, 0)

				for row := range symbol.MarketMeasurements(solver) {
					rows = append(rows, row)
				}

				So(rows, ShouldResemble, []*Measurement{measurement})
			}

			_, manifoldFound := symbol.Measurements.Load("manifold")
			So(manifoldFound, ShouldBeFalse)

			_, found := symbol.Measurements.Load("resonance")
			So(found, ShouldBeFalse)
		})
	})
}

func TestSymbolAppendResonanceMeasurement(t *testing.T) {
	Convey("Given a market mark without new normalized signal readings", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		measurement := &ResonanceMeasurement{Tick: 7, Mark: 101.25}

		ready := symbol.AppendResonanceMeasurement(measurement)

		Convey("It should preserve the mark as predictor ground truth", func() {
			So(ready, ShouldBeTrue)
			rows := make([]*ResonanceMeasurement, 0)

			for row := range symbol.ResonanceMeasurements() {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []*ResonanceMeasurement{measurement})
		})
	})
}

func TestSymbolResonanceInputs(t *testing.T) {
	Convey("Given one source with a normalized observation", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		value := 0.5

		Convey("It should enqueue a competing reading immediately", func() {
			symbol.AppendMeasurement(&Measurement{
				Source: SourceHawkes,
				Symbol: "BTC/USD",
				Tick:   1,
				Metadata: map[string]float64{
					"last_price": 100,
				},
				Metrics: map[string]MetricSample{
					MetricKey(MetricEventCount, SideBuy): {Normalized: &value},
				},
			})

			rows := make([]*ResonanceMeasurement, 0)

			for row := range symbol.ResonanceMeasurements() {
				rows = append(rows, row)
			}

			So(rows, ShouldHaveLength, 1)
			So(rows[0].Mark, ShouldEqual, 100)
			So(rows[0].Readings, ShouldHaveLength, 1)
		})
	})
}

func TestSymbolAppendTicker(t *testing.T) {
	Convey("Given one ticker appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		ticker := kraken.TickerData{Symbol: "BTC/USD"}

		symbol.AppendTicker(ticker, TickerReceivers)

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
			So(found, ShouldBeTrue)
			_, found = symbol.tickers.Load(SourceToxicity)
			So(found, ShouldBeFalse)
		})
	})
}

func TestSymbolAppendTrade(t *testing.T) {
	Convey("Given one trade appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		trade := kraken.TradeData{Symbol: "BTC/USD"}

		symbol.AppendTrade(trade, TradeReceivers)

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

func TestSymbolDrainCut(t *testing.T) {
	Convey("Given a queue holding past rows, a future row, and more past rows", t, func() {
		symbol := NewSymbol("BTC/USD", nil)

		for range 2 {
			symbol.AppendTicker(kraken.TickerData{
				Symbol: "BTC/USD", Timestamp: time.Now().Add(-time.Hour),
			}, []SourceType{SourceCVD})
		}

		symbol.AppendTicker(kraken.TickerData{
			Symbol: "BTC/USD", Timestamp: time.Now().Add(time.Hour),
		}, []SourceType{SourceCVD})

		for range 2 {
			symbol.AppendTicker(kraken.TickerData{
				Symbol: "BTC/USD", Timestamp: time.Now().Add(-time.Minute),
			}, []SourceType{SourceCVD})
		}

		Convey("It should process the boundary row and leave the rest pending", func() {
			rows := make([]kraken.TickerData, 0)

			for row := range symbol.MarketTickers(SourceCVD) {
				rows = append(rows, row)
			}

			So(rows, ShouldHaveLength, 3)
			So(rows[2].Timestamp, ShouldHappenAfter, rows[1].Timestamp)
			So(symbol.Pending(), ShouldBeTrue)

			remaining := 0

			for range symbol.MarketTickers(SourceCVD) {
				remaining++
			}

			So(remaining, ShouldEqual, 2)
			So(symbol.Pending(), ShouldBeFalse)
		})
	})
}

func TestSymbolPending(t *testing.T) {
	Convey("Given a clean symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)

		So(symbol.Pending(), ShouldBeFalse)

		Convey("Appending rows marks it pending", func() {
			symbol.AppendTicker(kraken.TickerData{Symbol: "BTC/USD"}, TickerReceivers)
			So(symbol.Pending(), ShouldBeTrue)

			Convey("Draining every receiver clears it", func() {
				for _, source := range TickerReceivers {
					for range symbol.MarketTickers(source) {
					}
				}

				So(symbol.Pending(), ShouldBeFalse)
			})

			Convey("A partial drain leaves it pending", func() {
				symbol.AppendTicker(kraken.TickerData{Symbol: "BTC/USD"}, TickerReceivers)
				drained := 0

				for _, source := range TickerReceivers {
					for range symbol.MarketTickers(source) {
						drained++
						break
					}
				}

				So(drained, ShouldEqual, len(TickerReceivers))
				So(symbol.Pending(), ShouldBeTrue)
			})
		})

		Convey("Trades and Level3 rows count independently", func() {
			symbol.AppendTrade(kraken.TradeData{Symbol: "BTC/USD"}, TradeReceivers)
			symbol.AppendLevel3(kraken.Level3Data{Symbol: "BTC/USD"}, Level3Receivers)

			for _, source := range TradeReceivers {
				for range symbol.MarketTrades(source) {
				}
			}

			So(symbol.Pending(), ShouldBeTrue)

			for _, source := range Level3Receivers {
				for range symbol.MarketLevel3(source) {
				}
			}

			So(symbol.Pending(), ShouldBeFalse)
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
		symbol.AppendMeasurement(measurement)
	}
}

func BenchmarkSymbolAppendResonanceMeasurement(b *testing.B) {
	symbol := NewSymbol("BTC/USD", nil)
	measurement := &ResonanceMeasurement{Tick: 1, Mark: 100}
	b.ReportAllocs()

	for b.Loop() {
		symbol.AppendResonanceMeasurement(measurement)

		for range symbol.ResonanceMeasurements() {
		}
	}
}

func BenchmarkSymbolResonanceInputs(b *testing.B) {
	symbol := NewSymbol("BTC/USD", nil)
	value := 0.0

	for epoch := range 2 {
		value = float64(epoch)

		for _, source := range SignalSources {
			symbol.AppendMeasurement(&Measurement{
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
			symbol.AppendMeasurement(&Measurement{
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
		symbol.AppendTicker(ticker, TickerReceivers)

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
		symbol.AppendTrade(trade, TradeReceivers)

		for _, source := range TradeReceivers {
			for range symbol.MarketTrades(source) {
			}
		}
	}
}

func TestSymbolBookRevision(t *testing.T) {
	Convey("Given accepted Level 3 frames with surviving-order timestamps", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		firstAt := time.Unix(10, 0).UTC()
		latestOrderAt := firstAt.Add(time.Second)
		symbol.AppendLevel3(kraken.Level3Data{
			Symbol:    symbol.Symbol,
			Timestamp: firstAt,
			Bids: []kraken.Level3Order{{
				OrderID: "bid", Timestamp: latestOrderAt,
			}},
		}, Level3Receivers)
		symbol.AppendLevel3(kraken.Level3Data{
			Symbol:    symbol.Symbol,
			Timestamp: firstAt.Add(2 * time.Second),
			Bids: []kraken.Level3Order{{
				OrderID: "delete-old", Timestamp: firstAt,
			}},
		}, Level3Receivers)

		revision, observedAt := symbol.BookRevision()

		Convey("It should advance by accepted frame and never regress event time", func() {
			So(revision, ShouldEqual, uint64(2))
			So(observedAt, ShouldEqual, firstAt.Add(2*time.Second))
		})
	})
}
