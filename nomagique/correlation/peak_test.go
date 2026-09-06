package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
profileOf is a scan whose strongest reading sits where the caller says, so a
case names the answer it expects by construction rather than by restating
the search.
*/
func profileOf(strongestAt int) []Point {
	profile := make([]Point, 0, 5)

	for index := range 5 {
		magnitude := 0.1 * float64(index)

		if index == strongestAt {
			magnitude = -0.9
		}

		profile = append(profile, Point{X: float64(index), Y: magnitude})
	}

	return profile
}

func TestPeakNext(t *testing.T) {
	Convey("Given a Peak as a Primitive", t, func() {
		peak := NewPeak(core.From(Point{}))

		Convey("When I show it one profile", func() {
			So(core.To[Point](peak.Next(core.From(profileOf(2)))).X, ShouldEqual, 2)
		})

		Convey("When the strongest reading runs the other way", func() {
			So(core.To[Point](peak.Next(core.From(profileOf(2)))).Y, ShouldEqual, -0.9)
		})

		Convey("When I show it several profiles in one step", func() {
			So(core.To[Point](peak.Next(tests.NewRun(
				core.From(profileOf(1)),
				core.From([]Point{{X: 9, Y: 2}}),
			))).X, ShouldEqual, 9)
		})

		Convey("When a reading is poisoned", func() {
			So(math.IsNaN(core.To[Point](peak.Next(core.From(
				[]Point{{X: 1, Y: math.NaN()}, {X: 2, Y: 0.5}},
			))).Y), ShouldBeTrue)
		})

		Convey("When I show it an empty profile", func() {
			peak.Next(core.From(profileOf(3)))

			So(core.To[Point](peak.Next(core.From([]Point{}))).X, ShouldEqual, 3)
		})

		Convey("When I offer nothing", func() {
			peak.Next(core.From(profileOf(3)))

			So(core.To[Point](peak.Next(nil)).X, ShouldEqual, 3)
		})
	})
}
