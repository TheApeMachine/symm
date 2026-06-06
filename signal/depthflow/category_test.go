package depthflow

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestDepthflowReading(t *testing.T) {
	Convey("Given a perfectly balanced book", t, func() {
		category, evidence := depthflowReading("", 0, 0, true, 0)

		Convey("It should not turn a zero denominator into full confidence", func() {
			So(category, ShouldEqual, types.CategoryDenseNeutrality)
			So(evidence, ShouldAlmostEqual, 0.5, 1e-12)
		})
	})

	Convey("Given pathological imbalance inputs", t, func() {
		category, evidence := depthflowReading(
			reasonDepthSkeptic,
			131_996_665_001.0,
			0,
			false,
			0,
		)

		Convey("It should keep shift evidence on the unit interval", func() {
			So(category, ShouldEqual, types.CategorySpoofTrap)
			So(evidence, ShouldBeGreaterThan, 0)
			So(evidence, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}
