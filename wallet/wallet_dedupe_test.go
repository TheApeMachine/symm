package wallet

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestApplyFillRejectsEmptyExecKey(t *testing.T) {
	Convey("Given a fill without a dedupe key", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

		Convey("ApplyFill should fail closed", func() {
			applied := tradingWallet.ApplyFill("", "buy", "BTC", 1, 100, 100)

			So(applied, ShouldBeFalse)
			So(tradingWallet.Inventory["BTC"], ShouldEqual, 0)
		})
	})
}
