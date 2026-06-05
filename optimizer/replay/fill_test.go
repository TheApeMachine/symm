package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTakerFillWalksBook(t *testing.T) {
	Convey("Given a tape row with two ask levels", t, func() {
		costs := triggerTestCosts()
		measurement := perspectives.Measurement{
			Symbol: "BTC/EUR",
			Last:   100,
			Bid:    99,
			Ask:    100,
			BookAsks: []perspectives.BookLevel{
				{Price: 100, Qty: 1},
				{Price: 101, Qty: 1},
			},
		}

		fill, err := takerFill(costs, measurement, trading.Buy, 1.5, nil)

		Convey("It should VWAP through available depth", func() {
			So(err, ShouldBeNil)
			So(fill.price, ShouldAlmostEqual, 100.3333333333, 0.0000001)
			So(fill.depthCoverage, ShouldEqual, 1)
		})
	})
}

func TestTakerFillDepthShortfallPenalty(t *testing.T) {
	Convey("Given a thin ask book", t, func() {
		costs := triggerTestCosts()
		ledger := newReplayLedger(costs)
		measurement := perspectives.Measurement{
			Symbol: "BTC/EUR",
			Last:   100,
			Bid:    99,
			Ask:    100,
			BookAsks: []perspectives.BookLevel{
				{Price: 100, Qty: 0.1},
			},
		}

		ledger.openEntry(
			"BTC/EUR",
			trading.Buy,
			perspectives.Act{},
			measurement,
			nil,
			0,
			measurement.At,
		)

		Convey("It should refuse the entry and book a depth penalty", func() {
			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.depthBlocked, ShouldEqual, 1)
			So(ledger.realized, ShouldBeLessThan, 0)
		})
	})
}

func BenchmarkTakerFill(b *testing.B) {
	costs := triggerTestCosts()
	measurement := perspectives.Measurement{
		Symbol: "BTC/EUR",
		Last:   100,
		Bid:    99,
		Ask:    100,
		BookAsks: []perspectives.BookLevel{
			{Price: 100, Qty: 1},
			{Price: 101, Qty: 1},
		},
	}

	for b.Loop() {
		_, _ = takerFill(costs, measurement, trading.Buy, 1.5, nil)
	}
}
