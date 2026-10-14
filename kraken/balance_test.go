package kraken_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestBalanceMeasurementRoundTrip(t *testing.T) {
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

		Convey("ToMeasurement serializes wallet balances into structured metrics", func() {
			meas := original.ToMeasurement(987654321)
			So(meas, ShouldNotBeNil)
			So(meas.Label, ShouldEqual, "snapshot")
			So(meas.Source, ShouldEqual, "balances")
			So(meas.Raw(), ShouldBeNil)

			Convey("and NewBalanceFromMeasurement completely restores balance rows", func() {
				restored, err := kraken.NewBalanceFromMeasurement(meas)
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
