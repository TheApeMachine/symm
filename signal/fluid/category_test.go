package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestFluidReading(t *testing.T) {
	Convey("Given turbulent dominance", t, func() {
		category, evidence := fluidReading(0.1, 0.8, 0.2, 100)

		Convey("It should classify turbulent flow", func() {
			So(category, ShouldEqual, perspectives.CategoryTurbulent)
			So(evidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given low viscosity without turbulence", t, func() {
		category, _ := fluidReading(0.1, 0, 0.1, 10)

		Convey("It should classify viscous flow", func() {
			So(category, ShouldEqual, perspectives.CategoryViscous)
		})
	})
}
