package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestCorrelationCategory(t *testing.T) {
	Convey("Given correlation and variance", t, func() {
		Convey("It should classify systemic herd", func() {
			So(correlationCategory(0.9, 1e-4), ShouldEqual, engine.CatSystemicHerd)
		})

		Convey("It should classify divergent stress", func() {
			So(correlationCategory(-0.5, 1e-4), ShouldEqual, engine.CatDivergentStress)
		})

		Convey("It should classify decoupled alpha", func() {
			So(correlationCategory(0.2, 1e-4), ShouldEqual, engine.CatDecoupledAlpha)
		})
	})
}
