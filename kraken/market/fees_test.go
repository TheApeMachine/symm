package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestPairTakerFeePercent(t *testing.T) {
	convey.Convey("Given a tiered Kraken fee schedule", t, func() {
		pair := &Pair{
			Wsname: "BTC/USD",
			Fees: [][]float64{
				{0, 0.40},
				{50000, 0.35},
				{100000, 0.24},
			},
		}

		convey.Convey("It should select the highest tier not above volume", func() {
			fee, err := pair.TakerFeePercent(0)

			convey.So(err, convey.ShouldBeNil)
			convey.So(fee, convey.ShouldEqual, 0.40)

			fee, err = pair.TakerFeePercent(75000)

			convey.So(err, convey.ShouldBeNil)
			convey.So(fee, convey.ShouldEqual, 0.35)

			fee, err = pair.TakerFeePercent(250000)

			convey.So(err, convey.ShouldBeNil)
			convey.So(fee, convey.ShouldEqual, 0.24)
		})
	})

	convey.Convey("Given a pair without a fee schedule", t, func() {
		pair := &Pair{Wsname: "BTC/USD"}

		convey.Convey("It should return an error", func() {
			_, err := pair.TakerFeePercent(0)

			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
