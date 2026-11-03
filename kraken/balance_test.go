package kraken_test

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestBalanceJSONRoundTrip(t *testing.T) {
	Convey("Given a kraken.Balance with asset rows", t, func() {
		btcBal, _ := decimal.NewFromString("1.5")
		usdBal, _ := decimal.NewFromString("1000.50")
		btcPtr := &btcBal
		usdPtr := &usdBal

		original := &kraken.Balance{
			Channel: "balances",
			Type:    "snapshot",
			Data: []kraken.BalanceData{
				{
					Asset:     "BTC",
					Balance:   btcPtr,
					Available: btcPtr,
				},
				{
					Asset:     "USD",
					Balance:   usdPtr,
					Available: usdPtr,
				},
			},
		}

		Convey("sonic.Marshal round-trips wallet balances", func() {
			raw, err := sonic.Marshal(original)
			So(err, ShouldBeNil)

			var decoded kraken.Balance
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)
			So(decoded.Channel, ShouldEqual, "balances")
			So(decoded.Type, ShouldEqual, "snapshot")
			So(decoded.Data, ShouldHaveLength, 2)
			So(decoded.Data[0].Asset, ShouldEqual, "BTC")
			So(decoded.Data[0].Balance.String(), ShouldEqual, btcBal.String())
			So(decoded.Data[1].Asset, ShouldEqual, "USD")
			So(decoded.Data[1].Available.String(), ShouldEqual, usdBal.String())
		})
	})
}
