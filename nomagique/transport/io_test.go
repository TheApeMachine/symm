package transport_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestIONext(t *testing.T) {
	Convey("IO connects the output of its input primitive to its output primitive", t, func() {
		input := sequence.NewValue(2.0, -3.0)
		pipe := transport.NewIO[float64](calculus.NewSquare(), calculus.NewNegate())
		So(tests.CollectSeq[float64](pipe.Next(input)), ShouldResemble, []float64{-4, -9})
		So(pipe.Error(), ShouldBeNil)

		Convey("Reversing the endpoints reverses the operation order", func() {
			reverse := transport.NewIO[float64](calculus.NewNegate(), calculus.NewSquare())
			So(tests.CollectSeq[float64](reverse.Next(sequence.NewValue(2.0, -3.0))), ShouldResemble, []float64{4, 9})
		})

		Convey("An upstream failure remains visible at the connection", func() {
			pipe := transport.NewIO[float64](sequence.NewGather[float64]([]int{1}), sequence.NewTail[float64](1))
			So(tests.CollectSeq[[]float64](pipe.Next(sequence.NewValue([]float64{2}))), ShouldBeEmpty)
			So(errors.Is(pipe.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("A downstream failure remains visible at the connection", func() {
			pipe := transport.NewIO[float64](sequence.NewTail[float64](1), sequence.NewGather[float64]([]int{1}))
			So(tests.CollectSeq[[]float64](pipe.Next(sequence.NewValue([]float64{2, 5}))), ShouldBeEmpty)
			So(errors.Is(pipe.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}

func BenchmarkIONext(b *testing.B) {
	pipe := transport.NewIO[float64](calculus.NewSquare(), calculus.NewNegate())
	values := []float64{1, 2, 3, 4}
	input := sequence.NewValue(values...)
	b.ReportAllocs()
	for b.Loop() {
		values[0], values[1], values[2], values[3] = 1, 2, 3, 4
		count := 0
		for range pipe.Next(input) {
			count++
		}
		if count != 4 {
			b.Fatal(count)
		}
	}
}
