package depthflow

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestDepthSymbolMeasureSpreadBPS(t *testing.T) {
	Convey("Given a verified book and ticker", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		state, err := NewDepthSymbol(symbol)
		So(err, ShouldBeNil)

		fixture := symbolBookFixture{symbol: symbol}
		state.ApplyBook(fixture.snapshot(99, 8, 101, 4))
		state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

		measurement, _, err := state.Measure()

		Convey("It should publish spread in basis points from the book", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldNotBeEmpty)
			So(measurement.SpreadBPS, ShouldBeGreaterThan, 0)
		})
	})
}

func TestDepthSymbolMeasureBalancedBookConfidence(t *testing.T) {
	Convey("Given a verified balanced book", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		state, err := NewDepthSymbol(symbol)
		So(err, ShouldBeNil)

		fixture := symbolBookFixture{symbol: symbol}
		state.ApplyBook(fixture.snapshot(99, 5, 101, 5))
		state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

		measurement, standout, err := state.Measure()

		Convey("It should publish neutral depth without saturated confidence", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, types.CategoryDenseNeutrality)
			So(measurement.Confidence, ShouldAlmostEqual, 0.5, 1e-12)
			So(standout, ShouldAlmostEqual, measurement.Confidence, 1e-12)
		})
	})
}

func TestDepthSymbolMeasureTradePressureFallback(t *testing.T) {
	Convey("Given trade pressure without a ready book", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		state, err := NewDepthSymbol(symbol)
		So(err, ShouldBeNil)

		_, pushErr := state.PushTradePressure(0.8)
		So(pushErr, ShouldBeNil)

		state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

		measurement, _, err := state.Measure()

		Convey("It should fall back to trade-pressure measurement", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, types.SourceDepthFlow)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
		})
	})
}
