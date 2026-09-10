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
	Convey("Median queries with different ordering and exceptional values", t, func() {
		for _, example := range []struct {
			values []float64
			want   float64
		}{
			{[]float64{9, -7, 3}, 3},
			{[]float64{9, -7, 3, -1}, 1},
			{[]float64{math.Inf(-1), 2, math.Inf(1)}, 2},
			{[]float64{math.Copysign(0, -1)}, 0},
			{[]float64{17}, 17},
		} {
			op := equation.NewMedian[float64]()
			result := tests.CollectSeq(op.Next(transport.Values(example.values...)))
			So(op.Error(), ShouldBeNil)
			So(len(result), ShouldEqual, 1)
			So(result[0], ShouldEqual, example.want)
		}

		Convey("An empty run is a shape error, not a previous median", func() {
			op := equation.NewMedian[float64]()
			_, err := transport.Evaluate(op, transport.Values[float64]())
			So(err, ShouldNotBeNil)
		})
	})
}
