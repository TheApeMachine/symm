package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/ring"
)

func feedTrades(state *CausalSymbol, base time.Time, prices []float64) {
	for index, price := range prices {
		_ = state.FeedTrade(market.TradeUpdate{
			Timestamp: base.Add(time.Duration(index) * time.Second),
			Price:     price,
			Qty:       1,
			Side:      "buy",
		})
	}
}

func TestAdaptiveContagionFastSpike(t *testing.T) {
	Convey("Given tier readings where the fast window leads the slow baseline", t, func() {
		history := ring.NewFloatRing(8)

		for range 6 {
			history.Push(0.02)
		}

		value := adaptiveContagion(contagionTier{fast: 0.95, medium: 0.7, slow: 0.3}, &history)

		Convey("It should surface the fast coupling", func() {
			So(value, ShouldEqual, 0.95)
		})
	})
}

func TestAdaptiveContagionThreshold(t *testing.T) {
	Convey("Given tier readings", t, func() {
		history := ring.NewFloatRing(8)

		Convey("A fast spike above the slow baseline should return the fast reading", func() {
			for range 6 {
				history.Push(0.01)
			}

			value := adaptiveContagion(contagionTier{fast: 0.9, medium: 0.5, slow: 0.2}, &history)

			So(value, ShouldEqual, 0.9)
		})
	})
}

func TestHYVolatilityReset(t *testing.T) {
	Convey("Given a symbol with a violent print", t, func() {
		state := NewCausalSymbol()
		base := time.Unix(1_700_000_000, 0)
		calm := []float64{100, 100.1, 99.9, 100, 100.05, 99.95, 100, 100.02, 99.98, 100, 100.01, 99.99, 100, 100, 100, 100}

		feedTrades(state, base, calm)
		before := state.HYWindowSnapshot().fast.len()

		feedTrades(state, base.Add(20*time.Second), []float64{80})
		after := state.HYWindowSnapshot().fast.len()

		Convey("It should trim stale intervals after a volatility shock", func() {
			So(after, ShouldBeLessThan, before)
		})
	})
}
