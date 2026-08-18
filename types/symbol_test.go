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

		Convey("It should expose the row to every consuming solver", func() {
			for _, solver := range []string{"category", "graph", "resonance", "manifold"} {
				rows := make([]*Measurement, 0)

				for row := range symbol.MarketMeasurements(solver) {
					rows = append(rows, row)
				}

				So(rows, ShouldResemble, []*Measurement{measurement})
			}
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
