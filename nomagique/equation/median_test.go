package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestMedianNext(t *testing.T) {
	Convey("Given repeated median queries with different ordering and exceptional values", t, func() {
		median := equation.NewMedian()

		for _, example := range []struct {
			values []float64
			want   float64
		}{
			{[]float64{9, -7, 3}, 3},
			{[]float64{9, -7, 3, -1}, 1},
			{[]float64{math.Inf(-1), 2, math.Inf(1)}, 2},
			{[]float64{1, 2, 3, math.NaN(), 5}, math.NaN()},
			{[]float64{math.Inf(-1), math.Inf(1)}, math.NaN()},
			{[]float64{math.Copysign(0, -1)}, 0},
			{[]float64{17}, 17},
		} {
			result := tests.Drain(t, median, values(example.values...))
			So(median.Error(), ShouldBeNil)
			So(len(result), ShouldEqual, 1)
			tests.EqualNumber(t, result[0], example.want)
		}

		Convey("An empty run cannot reuse the previous median", func() {
			_, err := transport.Evaluate[float64](median, transport.NewIO())
			So(err, ShouldNotBeNil)
		})
	})
}
