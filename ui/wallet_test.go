package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/user"
)

func TestWalletFrame(t *testing.T) {
	Convey("Given USD quote currency config", t, func() {
		viper.Set("market.quote_currency", "USD")

		frame := WalletFrame(user.Balances{
			Asset: []user.Balance{{
				Asset:   "USD",
				Balance: 200,
			}},
		})

		Convey("It should publish a wallet event with the configured currency", func() {
			So(frame["event"], ShouldEqual, "wallet")
			So(frame["balance"], ShouldEqual, 200)
			So(frame["currency"], ShouldEqual, "USD")
		})
	})
}
