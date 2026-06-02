package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestExhaustReading(t *testing.T) {
	Convey("Given dominant book thinning", t, func() {
		category, evidence := exhaustReading(0.9, 0.1, 0.1, 0.1)

		Convey("It should classify mechanical collapse", func() {
			So(category, ShouldEqual, perspectives.CategoryMechanicalCollapse)
			So(evidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given dominant spread widening", t, func() {
		category, _ := exhaustReading(0.1, 0.9, 0.1, 0.1)

		Convey("It should classify fragile expansion", func() {
			So(category, ShouldEqual, perspectives.CategoryFragileExpansion)
		})
	})
}
