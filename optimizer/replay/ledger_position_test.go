package replay

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestReplayLedgerFundableSymbol(t *testing.T) {
	convey.Convey("Given a replay ledger funded in EUR", t, func() {
		costs := triggerTestCosts()
		costs.WalletCurrency = "EUR"
		ledger := newReplayLedger(costs)

		convey.Convey("It should only fund pairs with the wallet quote currency", func() {
			convey.So(ledger.fundableSymbol("BTC/EUR"), convey.ShouldBeTrue)
			convey.So(ledger.fundableSymbol("ETH/BTC"), convey.ShouldBeFalse)
		})
	})
}
