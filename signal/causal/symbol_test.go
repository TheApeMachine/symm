package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestCausalSymbolMeasureZeroChangePct(t *testing.T) {
	Convey("Given macro drift without change_pct", t, func() {
		state := NewCausalSymbol()
		state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
			Bid:    49990,
			Ask:    50010,
		})

		Convey("It should measure from macro momentum without requiring change_pct", func() {
			reading, err := state.Measure(0.02, 0.5, time.Now())

			So(err, ShouldBeNil)
			So(reading.Strength, ShouldBeGreaterThan, 0)
		})
	})
}
