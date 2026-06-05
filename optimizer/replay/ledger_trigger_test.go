package replay

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestReplayLedgerArmOffset(t *testing.T) {
	convey.Convey("Given a replay ledger with an open long", t, func() {
		ledger := newReplayLedger(triggerTestCosts())
		ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})

		convey.Convey("It should not arm a dynamic trailing stop without volatility", func() {
			offset, ok := ledger.armOffset(
				"BTC/EUR",
				perspectives.Act{Type: perspectives.ActionTrailingStop},
			)

			convey.So(ok, convey.ShouldBeFalse)
			convey.So(offset, convey.ShouldEqual, 0)
		})

		convey.Convey("It should arm a dynamic trailing stop from realized volatility", func() {
			ledger.observeSymbolPrice("BTC/EUR", 101)
			ledger.observeSymbolPrice("BTC/EUR", 102)

			offset, ok := ledger.armOffset(
				"BTC/EUR",
				perspectives.Act{Type: perspectives.ActionTrailingStop},
			)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(offset, convey.ShouldBeGreaterThan, 0)
		})
	})
}
