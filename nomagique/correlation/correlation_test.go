package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
estimate arranges the whole dependence pipeline the way a caller does: two
paths retain what they saw, their returns are differenced out, the returns
are paired in time, and the pairing is normalized by what each path did.

It is here rather than in each case because it is the composition under test,
and a case that rebuilt it would be testing its own wiring.
*/
func estimate(left, right []Observation, shift float64) core.Primitive {
	leftReturns := NewReturns(core.From([]Interval(nil))).Next(
		NewPath(core.From([]Observation(nil))).Next(core.From(left)),
	)

	rightReturns := NewReturns(core.From([]Interval(nil))).Next(
		NewPath(core.From([]Observation(nil))).Next(core.From(right)),
	)

	return Correlation(
		NewOverlap(rightReturns, core.From(shift)).Next(leftReturns),
		NewEnergy(core.From(0.0)).Next(leftReturns),
		NewEnergy(core.From(0.0)).Next(rightReturns),
	)
}

/*
opposed is a path that mirrors the shared observation run: it falls by
exactly what the shared run rises by, so it is perfectly related to it and
perfectly against it.
*/
func opposed(count int) []Observation {
	run := make([]Observation, 0, count)

	for index := range count {
		run = append(run, Observation{
			Nanos: int64(index) * int64(NanosPerSecond),
			Value: math.Pow(0.5, float64(index)),
		})
	}

	return run
}

func TestCorrelation(t *testing.T) {
	Convey("Given two paths related through Correlation", t, func() {
		Convey("When a path is estimated against itself", func() {
			So(core.To[float64](estimate(observations(6), observations(6), 0)),
				ShouldAlmostEqual, 1, 1e-9)
		})

		Convey("When a path is estimated against its mirror", func() {
			So(core.To[float64](estimate(observations(6), opposed(6), 0)),
				ShouldAlmostEqual, -1, 1e-9)
		})

		Convey("When the paths never coincided in time", func() {
			So(core.To[float64](
				estimate(observations(6), observations(6), 1000*NanosPerSecond),
			), ShouldEqual, 0)
		})

		Convey("When a path holds no returns at all", func() {
			So(core.To[float64](estimate(observations(6), nil, 0)), ShouldEqual, 0)
		})

		Convey("When a path is poisoned", func() {
			poisoned := append(observations(5), Observation{
				Nanos: 5 * int64(NanosPerSecond), Value: math.Inf(1),
			})

			So(math.IsNaN(core.To[float64](estimate(poisoned, observations(6), 0))),
				ShouldBeTrue)
		})
	})
}
