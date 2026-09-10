package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewArgmax(t *testing.T) {
	Convey("Empty runs emit nothing and ties retain the first ordinal", t, func() {
		op := equation.NewArgmax[float64]()

		for _, sample := range []struct {
			values []float64
			index  int
			value  float64
		}{
			{nil, 0, 0},
			{[]float64{1, 9, 3}, 1, 9},
			{nil, 0, 0},
			{[]float64{4, 4}, 0, 4},
			{[]float64{-3, -1, -2}, 1, -1},
		} {
			output := tests.CollectSeq(op.Next(transport.Values(sample.values...)))
			So(op.Error(), ShouldBeNil)

			if len(sample.values) == 0 {
				So(output, ShouldBeEmpty)
				continue
			}

			So(output, ShouldHaveLength, 1)
			So(output[0].Index, ShouldEqual, sample.index)
			So(output[0].Value, ShouldEqual, sample.value)
		}
	})
}
