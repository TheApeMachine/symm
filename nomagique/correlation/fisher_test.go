package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestFisher(t *testing.T) {
	Convey("Given a correlation weighed through Fisher", t, func() {
		Convey("When there is no relationship at all", func() {
			So(core.To[float64](Fisher(core.From(0.0), core.From(103.0))),
				ShouldAlmostEqual, 1, 1e-12)
		})

		Convey("When a strong relationship rests on ample support", func() {
			So(core.To[float64](Fisher(core.From(0.8), core.From(103.0))),
				ShouldBeLessThan, 1e-6)
		})

		Convey("When the same relationship rests on almost nothing", func() {
			So(core.To[float64](Fisher(core.From(0.8), core.From(5.0))),
				ShouldBeGreaterThan, 0.05)
		})

		Convey("When support cannot state significance", func() {
			So(core.To[float64](Fisher(core.From(0.9), core.From(3.0))),
				ShouldAlmostEqual, 1, 1e-12)
			So(core.To[float64](Fisher(core.From(0.9), core.From(0.0))),
				ShouldAlmostEqual, 1, 1e-12)
		})

		Convey("When the same reading was the best of many looks", func() {
			searched := core.To[float64](Bonferroni(
				Fisher(core.From(0.5), core.From(103.0)), core.From(50.0),
			))

			So(searched, ShouldBeGreaterThan,
				core.To[float64](Fisher(core.From(0.5), core.From(103.0))))
			So(searched, ShouldBeLessThanOrEqualTo, 1)
		})

		Convey("When a search was wide enough to explain anything", func() {
			So(core.To[float64](Bonferroni(
				Fisher(core.From(0.5), core.From(103.0)), core.From(1e9),
			)), ShouldEqual, 1)
		})
	})
}
