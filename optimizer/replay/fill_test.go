package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestTakerFillWalksBook(t *testing.T) {
	Convey("Given a tape row with two ask levels", t, func() {
		costs := triggerTestCosts()
		measurement := types.Measurement{
			Symbol: "BTC/EUR",
			Last:   100,
			Bid:    99,
			Ask:    100,
			BookAsks: []types.BookLevel{
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

func TestTakerFillDepthShortfallPreflightReject(t *testing.T) {
	Convey("Given a thin ask book", t, func() {
		testconfig.Load(t)
		costs := triggerTestCosts()
		ledger := newReplayLedger(costs)
		now := time.Now().UTC()
		measurement := types.Measurement{
			Symbol: "BTC/EUR",
			Last:   100,
			Bid:    99,
			Ask:    100,
			At:     now,
			BookAsks: []types.BookLevel{
				{Price: 100, Qty: 0.1},
			},
		}

		ledger.openEntry(
			"BTC/EUR",
			trading.Buy,
			reasoning.Act{Type: reasoning.ActionMarket},
			measurement,
			nil,
			0,
			now,
			0,
		)

		Convey("It should refuse the entry at preflight like the desk", func() {
			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.preflightBlocked, ShouldEqual, 1)
		})
	})
}

func BenchmarkTakerFill(b *testing.B) {
	costs := triggerTestCosts()
	measurement := types.Measurement{
		Symbol: "BTC/EUR",
		Last:   100,
		Bid:    99,
		Ask:    100,
		BookAsks: []types.BookLevel{
			{Price: 100, Qty: 1},
			{Price: 101, Qty: 1},
		},
	}

	for b.Loop() {
		_, _ = takerFill(costs, measurement, trading.Buy, 1.5, nil)
	}
}
