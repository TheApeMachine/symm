package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSpacingsNext(t *testing.T) {
	Convey("Given consecutive timestamp differences within independent delivery runs", t, func() {
		spacing := equation.NewSpacings()
		// At modern Unix epochs, one nanosecond is below float64 timestamp precision.
		const epoch = int64(1788782400000000000)

		for _, example := range []struct {
			times []int64
			want  []float64
		}{
			{[]int64{epoch, epoch + 1, epoch + 9, epoch + 14}, []float64{1, 8, 5}},
			{[]int64{epoch + 20}, nil},
			{nil, nil},
			{[]int64{epoch + 4, epoch + 4, epoch + 2}, []float64{0, -2}},
		} {
			observations := make([]core.Primitive, len(example.times))

			for index, at := range example.times {
				observations[index] = core.Record(map[string]any{"at": at})
			}
			result := tests.Drain(t, spacing, transport.NewIO(observations...))
			So(spacing.Error(), ShouldBeNil)
			So(len(result), ShouldEqual, len(example.want))

			for index, want := range example.want {
				So(result[index], ShouldEqual, want)
			}
		}
	})
}

func BenchmarkSpacingsNext(b *testing.B) {
	observations := make([]core.Primitive, 128)

	for index := range observations {
		observations[index] = core.Record(map[string]any{"at": int64(index*index + index)})
	}
	input := transport.NewIO(observations...)
	spacing := transport.NewPipe(equation.NewSpacings(), equation.NewMedian())
	b.ReportAllocs()

	for b.Loop() {
		result := spacing.Next(input)

		if result == nil || core.To[float64](result) != 128 || spacing.Next(input) != nil {
			b.Fatal("unexpected median spacing", spacing.Error())
		}
	}
}
