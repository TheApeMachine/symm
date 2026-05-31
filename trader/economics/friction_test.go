package economics

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestRoundTripCostPctForFees(t *testing.T) {
	convey.Convey("Given maker entry and taker exit fees", t, func() {
		cost := RoundTripCostPctForFees(0.25, 0.40, 10)

		convey.Convey("It should include both fees and spread", func() {
			convey.So(cost, convey.ShouldAlmostEqual, 0.0075)
		})
	})
}

func BenchmarkRoundTripCostPctForFees(b *testing.B) {
	for b.Loop() {
		_ = RoundTripCostPctForFees(0.25, 0.40, 10)
	}
}
