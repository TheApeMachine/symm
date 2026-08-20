package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func TestSymbolNewSymbol(t *testing.T) {
	Convey("Given a symbol name", t, func() {
		symbol := NewSymbol("BTC/USD")

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
		previousFocus := Focus()
		SetFocus("BTC/USD")
		Reset(func() { SetFocus(previousFocus) })

		ui := transport.NewMapReduce[*UIFrame]([]string{"dashboard"}, nil, nil)
		symbol := NewSymbol("BTC/USD", ui)
		measurement := nmtypes.NewMeasurement("first", string(SourceHawkes), 0, 0)

		symbol.AppendMeasurement(measurement)

		Convey("It should expose the row to every consuming solver", func() {
			for _, solver := range []string{"category", "graph", "resonance", "manifold"} {
				rows := make([]*nmtypes.Measurement, 0)

				for row := range symbol.MarketMeasurements(solver) {
					rows = append(rows, row)
				}

				So(len(rows), ShouldBeGreaterThanOrEqualTo, 1)

				if len(rows) > 0 {
					So(rows[0].ID, ShouldEqual, "first")
				}
			}
		})

		Convey("It should publish the measurement to the dashboard transport", func() {
			frame, found := ui.Pop("dashboard")
			So(found, ShouldBeTrue)
			So(frame.Type, ShouldEqual, wire.FrameMeasurementsFrame)
			rows := frame.Value.(*wire.MeasurementsFrameT).Rows
			So(rows, ShouldHaveLength, 1)
			So(rows[0].Id, ShouldEqual, "first")
		})
	})

	Convey("Given a measurement appended outside the dashboard focus", t, func() {
		previousFocus := Focus()
		SetFocus("BTC/USD")
		Reset(func() { SetFocus(previousFocus) })

		ui := transport.NewMapReduce[*UIFrame]([]string{"dashboard"}, nil, nil)
		symbol := NewSymbol("ETH/USD", ui)
		measurement := nmtypes.NewMeasurement("unfocused", string(SourceHawkes), 0, 0)

		symbol.AppendMeasurement(measurement)

		Convey("It should retain the measurement without publishing a UI frame", func() {
			row, found := symbol.Measurements.Pop("audit")
			So(found, ShouldBeTrue)
			So(row.ID, ShouldEqual, "unfocused")

			_, found = ui.Pop("dashboard")
			So(found, ShouldBeFalse)
		})
	})
}

func TestSymbolAppendTicker(t *testing.T) {
	Convey("Given one ticker appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD")
		ticker := kraken.TickerData{Symbol: "BTC/USD"}

		symbol.AppendTicker(ticker)

		Convey("It should queue the ticker", func() {
			So(symbol.HasTickersFor(SourceLiquidity), ShouldBeTrue)
			So(symbol.HasTickersFor(SourceCVD), ShouldBeFalse)
			rows := make([]kraken.TickerData, 0)

			for row := range symbol.MarketTickers(SourceLiquidity) {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []kraken.TickerData{ticker})
			So(symbol.HasTickersFor(SourceLiquidity), ShouldBeFalse)
			So(symbol.HasTickersFor(SourceSentiment), ShouldBeTrue)
		})
	})
}

func TestSymbolAppendTrade(t *testing.T) {
	Convey("Given one trade appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD")
		trade := kraken.TradeData{Symbol: "BTC/USD"}

		symbol.AppendTrade(trade)

		Convey("It should queue the trade", func() {
			So(symbol.HasTradesFor(SourceCVD), ShouldBeTrue)
			So(symbol.HasTradesFor(SourceDepthFlow), ShouldBeFalse)
			So(symbol.HasTradesFor(SourceToxicity), ShouldBeFalse)
			rows := make([]kraken.TradeData, 0)

			for row := range symbol.MarketTrades(SourceCVD) {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []kraken.TradeData{trade})
			So(symbol.HasTradesFor(SourceCVD), ShouldBeFalse)
			So(symbol.HasTradesFor(SourceHawkes), ShouldBeTrue)
		})
	})
}

func TestSymbolAppendLevel3(t *testing.T) {
	Convey("Given one Level 3 frame appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD")
		level3 := kraken.Level3Data{Symbol: "BTC/USD"}

		symbol.AppendLevel3(level3)

		Convey("It should queue the frame only for Level 3 consumers", func() {
			So(symbol.HasLevel3For(SourceDepthFlow), ShouldBeTrue)
			So(symbol.HasLevel3For(SourceToxicity), ShouldBeTrue)
			So(symbol.HasLevel3For(SourcePumpDump), ShouldBeFalse)
			rows := make([]kraken.Level3Data, 0)

			for row := range symbol.MarketLevel3(SourceDepthFlow) {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []kraken.Level3Data{level3})
		})
	})
}

func BenchmarkSymbolAppendMeasurement(b *testing.B) {
	previousFocus := Focus()
	SetFocus("BTC/USD")
	b.Cleanup(func() { SetFocus(previousFocus) })

	ui := transport.NewMapReduce[*UIFrame]([]string{"dashboard"}, nil, nil)
	measurement := nmtypes.NewMeasurement("benchmark", string(SourceHawkes), 0, 0)
	consumers := append(append([]string{}, LogicSourceStrings...), "audit")

	b.Run("focused", func(b *testing.B) {
		symbol := NewSymbol("BTC/USD", ui)
		b.ReportAllocs()

		for range b.N {
			symbol.AppendMeasurement(measurement)
			ui.Pop("dashboard")

			for _, consumer := range consumers {
				symbol.Measurements.Pop(consumer)
			}
		}
	})

	b.Run("unfocused", func(b *testing.B) {
		symbol := NewSymbol("ETH/USD", ui)
		b.ReportAllocs()

		for range b.N {
			symbol.AppendMeasurement(measurement)

			for _, consumer := range consumers {
				symbol.Measurements.Pop(consumer)
			}
		}
	})
}
