package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestPairTakerFeePercent(t *testing.T) {
	convey.Convey("Given a tiered Kraken fee schedule", t, func() {
		pair := &Pair{
			Fees: [][]float64{
				{0, 0.40},
				{50000, 0.35},
				{100000, 0.24},
			},
		}

		convey.Convey("It should select the highest tier not above volume", func() {
			convey.So(pair.TakerFeePercent(0, 0.40), convey.ShouldEqual, 0.40)
			convey.So(pair.TakerFeePercent(75000, 0.40), convey.ShouldEqual, 0.35)
			convey.So(pair.TakerFeePercent(250000, 0.40), convey.ShouldEqual, 0.24)
		})
	})
}
