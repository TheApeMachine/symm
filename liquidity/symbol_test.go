package liquidity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func TestLiquiditySymbolStateObserveTicker(t *testing.T) {
	Convey("Given a liquidity symbol state", t, func() {
		state := newSymbolState(asset.Pair{Wsname: "ALT/EUR", Quote: "EUR"})
		state.observeTicker(market.TickerRow{
			Symbol: "ALT/EUR",
			Last:   10,
			Bid:    9.9,
			Ask:    10.1,
			Volume: 1000,
		})

		snapshot := state.snapshot()

		Convey("It should track quote volume and top of book", func() {
			So(snapshot.dailyQuoteVol, ShouldAlmostEqual, 10000, 1e-9)
			So(snapshot.last, ShouldAlmostEqual, 10, 1e-12)
			So(snapshot.bid, ShouldAlmostEqual, 9.9, 1e-12)
			So(snapshot.ask, ShouldAlmostEqual, 10.1, 1e-12)
		})
	})
}

func TestLiquiditySymbolStateApplyFeedback(t *testing.T) {
	Convey("Given forecast feedback", t, func() {
		state := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})

		err := state.applyFeedback(0.02, -0.01)

		Convey("It should update the per-symbol forecast", func() {
			So(err, ShouldBeNil)
		})
	})
}

func TestLiquidityConfidenceZeroScore(t *testing.T) {
	Convey("Given a zero illiquidity score", t, func() {
		liquidity := &Liquidity{}

		Convey("It should return zero confidence", func() {
			So(liquidity.confidenceFromScore(0, []float64{0.2}), ShouldEqual, 0)
		})
	})
}

func BenchmarkLiquiditySymbolSnapshot(b *testing.B) {
	state := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	state.observeTicker(market.TickerRow{Last: 10, Volume: 1000})

	for b.Loop() {
		_ = state.snapshot()
	}
}
