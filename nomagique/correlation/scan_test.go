package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
varied is a path whose every return differs from the last, so that a shift
against it is answerable: a path that repeats the same move forever
correlates with itself equally well at every offset, and would make a search
look broken when it is the path that says nothing.
*/
func varied(count int) []Observation {
	run := make([]Observation, 0, count)

	for index := range count {
		run = append(run, Observation{
			Nanos: int64(index) * int64(NanosPerSecond),
			Value: math.Exp(math.Sin(float64(index))),
		})
	}

	return run
}

/*
trailing is a path repeating what the varied path did some seconds earlier,
which is the relationship a search is supposed to find and place. Before it
has anything to repeat it stands still, so its opening returns are zero
rather than invented.
*/
func trailing(count, seconds int) []Observation {
	leader := varied(count)
	run := make([]Observation, 0, count)

	for index := range count {
		run = append(run, Observation{
			Nanos: int64(index) * int64(NanosPerSecond),
			Value: leader[max(index-seconds, 0)].Value,
		})
	}

	return run
}

/*
scanned arranges the search the way a caller does: the step is the paths' own
median sampling interval, and the range is whatever the caller is willing to
look over.
*/
func scanned(left, right []Observation, span float64) core.Primitive {
	leftPath := NewPath(core.From([]Observation(nil))).Next(core.From(left))
	rightPath := NewPath(core.From([]Observation(nil))).Next(core.From(right))

	spacing := NewMedian(core.From(0.0)).Next(
		NewSpacings(core.From([]float64(nil))).Next(leftPath),
	)

	return NewScan(
		NewReturns(core.From([]Interval(nil))).Next(rightPath),
		spacing, core.From(span),
	).Next(NewReturns(core.From([]Interval(nil))).Next(leftPath))
}

func TestScanNext(t *testing.T) {
	Convey("Given a Scan as a Primitive", t, func() {
		Convey("When a path is scanned against itself", func() {
			profile := scanned(varied(16), varied(16), 4)

			So(len(core.To[[]Point](profile)), ShouldEqual, 9)
			So(core.To[Point](NewPeak(core.From(Point{})).Next(profile)).X,
				ShouldEqual, 0)
			So(core.To[Point](NewPeak(core.From(Point{})).Next(profile)).Y,
				ShouldAlmostEqual, 1, 1e-9)
		})

		Convey("When one path repeats what the other did earlier", func() {
			profile := scanned(varied(24), trailing(24, 2), 4)

			So(core.To[Point](NewPeak(core.From(Point{})).Next(profile)).X,
				ShouldEqual, 2*NanosPerSecond)
		})

		Convey("When the peak is read for its shape", func() {
			profile := scanned(varied(24), trailing(24, 2), 4)

			So(core.To[float64](NewProminence(core.From(0.0)).Next(profile)),
				ShouldBeGreaterThan, 0)
			So(core.To[float64](NewCurvature(core.From(0.0)).Next(profile)),
				ShouldBeGreaterThan, 0)
		})

		Convey("When nothing is looked over", func() {
			So(len(core.To[[]Point](scanned(varied(8), varied(8), 0))),
				ShouldEqual, 1)
		})

		Convey("When I offer nothing", func() {
			scan := NewScan(
				core.From([]Interval(nil)), core.From(0.0), core.From(0.0),
			)

			So(len(core.To[[]Point](scan.Next(nil))), ShouldEqual, 1)
		})
	})
}
