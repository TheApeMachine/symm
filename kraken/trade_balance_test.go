package kraken_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestTradeBalanceResultMeasurementRoundTrip(t *testing.T) {
	Convey("Given a valid kraken.TradeBalanceResult and wallet", t, func() {
		unrealized, _ := decimal.NewFromString("-0.32")
		equity, _ := decimal.NewFromString("199.68")
		cash, _ := decimal.NewFromString("160.14")
		zamaAmount, _ := decimal.NewFromString("517.71")

		tb := &kraken.TradeBalanceResult{
			UnrealizedPnL: unrealized,
			Equity:        equity,
			AvailableCash: cash,
		}

		wallet := &kraken.Balance{
			Data: []kraken.BalanceData{
				{
					Asset:     "USD",
					Balance:   cash,
					Available: cash,
				},
				{
					Asset:     "ZAMA",
					Balance:   zamaAmount,
					Available: zamaAmount,
				},
			},
		}

		Convey("ToValue serializes trade balance and wallet into Value", func() {
			val := tb.ToValue(wallet, "USD", 123456789)
			So(val, ShouldNotBeNil)
			So(val.Scope, ShouldEqual, "USD")
			So(val.Role, ShouldEqual, "trade_balance")
			So(val.Epoch, ShouldEqual, uint64(123456789))
		})
	})
}
