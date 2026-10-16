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

			Convey("and NewBalanceFromValue completely restores balance rows", func() {
				restored, err := kraken.NewBalanceFromValue(val)
				So(err, ShouldBeNil)
				So(restored, ShouldNotBeNil)
				So(len(restored.Data), ShouldEqual, 2)

				dataMap := make(map[string]kraken.BalanceData)
				for _, row := range restored.Data {
					dataMap[row.Asset] = row
				}

				So(dataMap["BTC"].Balance.String(), ShouldEqual, "1.5")
				So(dataMap["USD"].Balance.String(), ShouldEqual, "1000.50")
			})
		})
	})
}
