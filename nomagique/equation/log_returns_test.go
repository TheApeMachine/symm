package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
)

func TestLogReturnsLoad(t *testing.T) {
	Convey("Given uneven nanosecond price paths", t, func() {
		returns := equation.LogReturns{}
		times := []int64{1700000000000000000, 1700000000000000007, 1700000000000000020, 1700000000000000029}
		values := []float64{1, math.E, 1, math.Exp(2)}
		observations := make([]equation.Price, len(times))

		for index := range times {
			observations[index] = equation.Price{At: times[index], Value: values[index]}
		}

		So(returns.Load(observations), ShouldBeNil)
		So(returns.Energy, ShouldAlmostEqual, 6)

		Convey("Every signed return retains its exact adjacent coordinates", func() {
			So(len(returns.Intervals), ShouldEqual, 3)

			for index, expected := range []float64{1, -1, 2} {
				So(returns.Intervals[index].Value, ShouldAlmostEqual, expected)
				So(returns.Intervals[index].From, ShouldEqual, times[index])
				So(returns.Intervals[index].To, ShouldEqual, times[index+1])
			}

			So(returns.MedianEnergyRate(), ShouldAlmostEqual, 1/7e-9, 1e-6)
		})

		Convey("A shorter run replaces all previous statistics", func() {
			So(returns.Load(observations[:2]), ShouldBeNil)
			So(returns.Energy, ShouldAlmostEqual, 1)
			So(len(returns.Intervals), ShouldEqual, 1)
		})
	})
}
