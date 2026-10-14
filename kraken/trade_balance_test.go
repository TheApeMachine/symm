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

		Convey("ToMeasurement serializes trade balance and wallet into structured metrics", func() {
			meas := tb.ToMeasurement(wallet, "USD", 123456789)
			So(meas, ShouldNotBeNil)
			So(meas.Label, ShouldEqual, "balance")
			So(meas.Source, ShouldEqual, "balance")
			So(meas.Raw(), ShouldBeNil)

			Convey("and NewTradeBalanceResultFromMeasurement reconstructs equity, unrealized, and cash", func() {
				restored, err := kraken.NewTradeBalanceResultFromMeasurement(meas)
				So(err, ShouldBeNil)
				So(restored, ShouldNotBeNil)
				So(restored.Equity.String(), ShouldEqual, tb.Equity.String())
				So(restored.UnrealizedPnL.String(), ShouldEqual, tb.UnrealizedPnL.String())
				So(restored.AvailableCash.String(), ShouldEqual, tb.AvailableCash.String())
			})
		})
	})
}
