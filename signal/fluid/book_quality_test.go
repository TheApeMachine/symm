package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/toxicity"
)

func TestFluidSymbolExcludesToxicImbalance(t *testing.T) {
	Convey("Given a fluid symbol and a toxic near-touch bid", t, func() {
		t.Cleanup(viper.Reset)
		toxicity.ResetDefault()

		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)

		now := time.Now()
		symbol := "ETH/EUR"
		mid := 100.0
		toxicBid := mid

		tracker := toxicity.Default()
		tracker.ObserveMid(symbol, market.Pair{}, mid)
		tracker.ApplyOrder(symbol, market.Pair{}, "add", "base-bid", toxicity.SideBid, 98, 100, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "add", "spoof-bid", toxicity.SideBid, toxicBid, 15, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "delete", "spoof-bid", toxicity.SideBid, toxicBid, 15, now, now)
		So(tracker.IsToxic(symbol, toxicBid, now), ShouldBeTrue)

		fluidSymbol, err := NewFluidSymbol(symbol, fluidTestClassifier())
		So(err, ShouldBeNil)

		So(fluidSymbol.FeedTicker(market.TickerUpdate{
			Symbol: symbol, Last: mid, Bid: toxicBid, Ask: 101, Volume: 1000,
		}), ShouldBeNil)
		So(fluidSymbol.FeedBook(symbolBookFixture{symbol: symbol}.snapshot(toxicBid, 50, 101, 5)), ShouldBeNil)

		row := fluidSymbol.Row()

		Convey("It should exclude toxic resting size from divergence", func() {
			So(row, ShouldNotBeNil)

			div, ok := row["div"].(float64)

			So(ok, ShouldBeTrue)
			So(div, ShouldAlmostEqual, -1, 0.0001)
		})
	})
}

func TestFluidSymbolVorticityWithoutNearTouchToxicity(t *testing.T) {
	Convey("Given near-touch toxicity on the shared tracker", t, func() {
		t.Cleanup(viper.Reset)
		toxicity.ResetDefault()

		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)

		now := time.Now()
		symbol := "ETH/EUR"
		price := 100.0

		tracker := toxicity.Default()
		tracker.ObserveMid(symbol, market.Pair{}, price)
		tracker.ApplyOrder(symbol, market.Pair{}, "add", "base-bid", toxicity.SideBid, 98, 100, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "add", "spoof-bid", toxicity.SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "delete", "spoof-bid", toxicity.SideBid, price, 15, now, now)

		fluidSymbol, err := NewFluidSymbol(symbol, fluidTestClassifier())
		So(err, ShouldBeNil)

		So(fluidSymbol.FeedTicker(market.TickerUpdate{
			Symbol: symbol, Last: price, Bid: 99, Ask: 101, Volume: 1000,
		}), ShouldBeNil)

		for range 12 {
			So(fluidSymbol.FeedTradeSide(now, 1, "buy"), ShouldBeNil)
		}

		So(fluidSymbol.FeedBook(symbolBookFixture{symbol: symbol}.snapshot(99, 10, 101, 10)), ShouldBeNil)

		row := fluidSymbol.Row()

		Convey("It should withhold churn amplification while bluff liquidity is active", func() {
			So(row, ShouldNotBeNil)

			vort, ok := row["vort"].(float64)

			So(ok, ShouldBeTrue)
			So(vort, ShouldEqual, fluidSymbol.buyPressure)
		})
	})
}
