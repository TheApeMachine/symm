package trader

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/user"
)

func TestWalletCapitalProvider(t *testing.T) {
	Convey("Given a wallet snapshot", t, func() {
		provider := NewWalletCapitalProvider()
		provider.ApplyBalances(user.Balances{
			Currency: "USD",
			Balance:  180,
		})

		balance, err := provider.AvailableQuoteBalance(context.Background(), "USD")

		Convey("It should expose the latest quote balance", func() {
			So(err, ShouldBeNil)
			So(balance, ShouldEqual, 180)
		})
	})
}
