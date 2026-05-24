package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestWalletCreditAndDebitBase(t *testing.T) {
	convey.Convey("Given a wallet with spot inventory", t, func() {
		wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

		convey.Convey("It should credit and debit base qty for one symbol", func() {
			err := wallet.CreditBase("BTC/EUR", 0.01)
			convey.So(err, convey.ShouldBeNil)
			convey.So(wallet.AvailableBase("BTC/EUR"), convey.ShouldEqual, 0.01)

			err = wallet.DebitBase("BTC/EUR", 0.01)
			convey.So(err, convey.ShouldBeNil)
			convey.So(wallet.AvailableBase("BTC/EUR"), convey.ShouldEqual, 0)
		})

		convey.Convey("It should reject debits larger than inventory", func() {
			err := wallet.CreditBase("BTC/EUR", 0.01)
			convey.So(err, convey.ShouldBeNil)

			err = wallet.DebitBase("BTC/EUR", 0.02)
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
