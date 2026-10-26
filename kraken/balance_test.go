package kraken_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestBalanceValueRoundTrip(t *testing.T) {
	Convey("Given a kraken.Balance with asset rows", t, func() {
		btcBal, _ := decimal.NewFromString("1.5")
		usdBal, _ := decimal.NewFromString("1000.50")

		original := &kraken.Balance{
			Channel: "balances",
			Type:    "snapshot",
			Data: []kraken.BalanceData{
				{
					Asset:     "BTC",
					Balance:   btcBal,
					Available: btcBal,
				},
				{
					Asset:     "USD",
					Balance:   usdBal,
					Available: usdBal,
				},
			},
		}

		Convey("ToValue serializes wallet balances into Value", func() {
			val := original.ToValue(987654321)
			So(val, ShouldNotBeNil)
			So(val.Origin, ShouldEqual, "kraken")
			So(val.Role, ShouldEqual, "snapshot")
			So(val.Scope, ShouldEqual, "balances")
			So(val.Epoch, ShouldEqual, uint64(987654321))
		})
	})
}
