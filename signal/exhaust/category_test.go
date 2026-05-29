package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestExhaustCategory(t *testing.T) {
	Convey("Given exit reasons", t, func() {
		Convey("It should map book thinning to mechanical collapse", func() {
			So(exhaustCategory("book_thinning"), ShouldEqual, engine.CatMechanicalCollapse)
		})

		Convey("It should map spread widen to fragile expansion", func() {
			So(exhaustCategory("spread_widen"), ShouldEqual, engine.CatFragileExpansion)
		})
	})
}

func TestExhaustMeasurement(t *testing.T) {
	Convey("Given thinning bid depth", t, func() {
		history := symbolHistoryFrom(
			[]float64{100, 95, 90, 40, 35},
			[]float64{10, 10, 10, 10, 10},
			[]float64{0.8, 0.75, 0.7, 0.2},
			[]float64{0.5, 0.4, 0.3, -0.1},
		)

		measurement, ok := exhaustMeasurement("BTC/EUR", history)

		Convey("It should emit a categorized measurement", func() {
			So(ok, ShouldBeTrue)
			So(measurement.Category, ShouldNotBeEmpty)
			So(measurement.Source, ShouldEqual, exhaustSource)
		})
	})
}
