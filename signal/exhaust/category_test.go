package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestExhaustReading(t *testing.T) {
	Convey("Given dominant book thinning", t, func() {
		category, evidence := exhaustReading(0.9, 0.1, 0.1, 0.1)

		Convey("It should classify mechanical collapse", func() {
			So(category, ShouldEqual, types.CategoryMechanicalCollapse)
			So(evidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given dominant spread widening", t, func() {
		category, _ := exhaustReading(0.1, 0.9, 0.1, 0.1)

		Convey("It should classify fragile expansion", func() {
			So(category, ShouldEqual, types.CategoryFragileExpansion)
		})
	})

	Convey("Given a lone exit mode (the others reading exactly 0)", t, func() {
		// The old formula returned (winner-0)/winner = 1.0 for ANY lone mode, so a
		// 2% thinning flicker reported the same certainty as a violent collapse.
		_, faint := exhaustReading(0.02, 0, 0, 0)
		_, violent := exhaustReading(2.0, 0, 0, 0)

		Convey("A faint flicker is honestly low-confidence, a violent one far higher", func() {
			So(faint, ShouldBeLessThan, 0.1)
			So(violent, ShouldBeGreaterThan, faint)
			So(violent, ShouldBeLessThan, 1) // intensity never saturates to exactly 1
		})
	})

	Convey("Given two modes firing close together", t, func() {
		_, lone := exhaustReading(0.8, 0, 0, 0)
		_, contested := exhaustReading(0.8, 0.7, 0, 0)

		Convey("The contested reading is less confident than the clean lone one", func() {
			So(contested, ShouldBeLessThan, lone) // purity term still bites when modes compete
		})
	})
}
