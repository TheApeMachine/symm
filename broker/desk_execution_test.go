package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

func TestEconomicEntryPrice(t *testing.T) {
	Convey("Given a buy fill with fees", t, func() {
		entry := economicEntryPrice(user.Execution{
			Side:        string(trading.Buy),
			FeeUsdEquiv: 0.26,
		}, 1, 100)

		Convey("It should include the fee in the per-unit entry", func() {
			So(entry, ShouldAlmostEqual, 100.26, 1e-9)
		})
	})

	Convey("Given a sell fill with fees", t, func() {
		entry := economicEntryPrice(user.Execution{
			Side:        string(trading.Sell),
			FeeUsdEquiv: 0.26,
		}, 1, 100)

		Convey("It should keep the fill price", func() {
			So(entry, ShouldEqual, 100)
		})
	})
}
