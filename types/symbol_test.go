package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
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
	})
}

func TestSymbolAppendTicker(t *testing.T) {
	Convey("Given one ticker appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		ticker := kraken.TickerData{Symbol: "BTC/USD"}

		symbol.AppendTicker(ticker)

		Convey("It should queue the ticker", func() {
			rows := make([]kraken.TickerData, 0)

			for row := range symbol.MarketTickers(SourceLiquidity) {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []kraken.TickerData{ticker})
		})
	})
}

func TestSymbolAppendTrade(t *testing.T) {
	Convey("Given one trade appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		trade := kraken.TradeData{Symbol: "BTC/USD"}

		symbol.AppendTrade(trade)

		Convey("It should queue the trade", func() {
			rows := make([]kraken.TradeData, 0)

			for row := range symbol.MarketTrades(SourceCVD) {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []kraken.TradeData{trade})
		})
	})
}
