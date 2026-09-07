package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestLogReturnsLoad(t *testing.T) {
	Convey("Given uneven nanosecond price paths", t, func() {
		returns := equation.LogReturns{}
		times := []int64{1700000000000000000, 1700000000000000007, 1700000000000000020, 1700000000000000029}
		values := []float64{1, math.E, 1, math.Exp(2)}
		observations := tests.Path(times, values)
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

		Convey("A shorter, singleton, and empty run replace all previous statistics", func() {
			So(returns.Load(observations[:2]), ShouldBeNil)
			So(returns.Energy, ShouldAlmostEqual, 1)
			So(len(returns.Intervals), ShouldEqual, 1)
			So(returns.Load(observations[:1]), ShouldBeNil)
			So(len(returns.Intervals), ShouldEqual, 0)
			So(returns.Energy, ShouldEqual, 0)
			So(returns.Load(nil), ShouldBeNil)
			So(returns.From, ShouldEqual, 0)
			So(math.IsNaN(returns.MedianEnergyRate()), ShouldBeTrue)
		})

		Convey("Duplicate or regressed coordinates are explicit shape failures", func() {
			for _, at := range []int64{times[0], times[0] - 1} {
				So(returns.Load(tests.Path([]int64{times[0], at}, []float64{1, 2})), ShouldNotBeNil)
			}
		})
	})
}

func TestNewLogReturns(t *testing.T) {
	Convey("Given independent delivery runs through the return primitive", t, func() {
		node := equation.NewLogReturns()
		input := tests.Values(tests.Path([]int64{0, 7, 20}, []float64{1, math.E, 1})...)
		first := tests.Drain(t, node, input)
		second := tests.Drain(t, node, input)
		So(node.Error(), ShouldBeNil)
		So(len(first), ShouldEqual, 2)
		So(len(second), ShouldEqual, 2)
		So(tests.Number(t, tests.Fields(t, first[0]), "value"), ShouldAlmostEqual, 1)
		So(tests.Number(t, tests.Fields(t, first[1]), "value"), ShouldAlmostEqual, -1)
		So(tests.Number(t, tests.Fields(t, second[0]), "value"), ShouldAlmostEqual, 1)
	})
}

func BenchmarkLogReturnsLoad(b *testing.B) {
	times, prices := make([]int64, 128), make([]float64, 128)
	for index := range times {
		times[index], prices[index] = int64(index*2), 100*math.Exp(.01*math.Sin(float64(index)))
	}
	observations := tests.Path(times, prices)
	returns := equation.LogReturns{}
	b.ReportAllocs()
	for b.Loop() {
		if err := returns.Load(observations); err != nil {
			b.Fatal(err)
		}
	}
}
