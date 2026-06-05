package replay

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestReplayLedgerObserveSymbolPrice(t *testing.T) {
	convey.Convey("Given a replay ledger price path", t, func() {
		ledger := newReplayLedger(triggerTestCosts())

		ledger.observeSymbolPrice("BTC/EUR", 100)
		ledger.observeSymbolPrice("BTC/EUR", 101)
		ledger.observeSymbolPrice("BTC/EUR", 101)
		ledger.observeSymbolPrice("BTC/EUR", 102)

		convey.Convey("It should store distinct prices and derive volatility", func() {
			convey.So(ledger.pricePaths["BTC/EUR"], convey.ShouldHaveLength, 3)
			convey.So(ledger.priceVolatility("BTC/EUR"), convey.ShouldBeGreaterThan, 0)
		})
	})
}
