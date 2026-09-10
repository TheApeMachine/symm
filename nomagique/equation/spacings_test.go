package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSpacingsNext(t *testing.T) {
	Convey("Consecutive timestamp differences within independent delivery runs", t, func() {
		const epoch = int64(1788782400000000000)
		op := equation.NewSpacings()

		for _, example := range []struct {
			times []int64
			want  []float64
		}{
			{[]int64{epoch, epoch + 1, epoch + 9, epoch + 14}, []float64{1, 8, 5}},
			{[]int64{epoch + 20}, nil},
			{nil, nil},
			{[]int64{epoch + 4, epoch + 4, epoch + 2}, []float64{0, -2}},
		} {
			stamps := make([]equation.Stamp, len(example.times))

			for index, at := range example.times {
				stamps[index] = equation.Stamp{At: at}
			}

			result := tests.CollectSeq(op.Next(transport.Values(stamps...)))
			So(op.Error(), ShouldBeNil)
			So(len(result), ShouldEqual, len(example.want))

			for index, want := range example.want {
				So(result[index], ShouldEqual, want)
			}
		}
	})
}
