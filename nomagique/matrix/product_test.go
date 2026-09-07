package matrix_test

import (
	"errors"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestProductNext(t *testing.T) {
	Convey("Given complete matrix operands", t, func() {
		Convey("Successive signed products preserve earlier results and the inputs", func() {
			node := matrix.NewProduct(store.NewGet("left"), store.NewGet("right"))
			left := [][]float64{{1, -2}, {3, 4}}
			right := [][]float64{{5, 6}, {7, 8}}
			first, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"left": left, "right": right}))
			So(err, ShouldBeNil)
			So(first, ShouldResemble, [][]float64{{-9, -10}, {43, 50}})
			second, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"left": right, "right": left}))
			So(err, ShouldBeNil)
			So(second, ShouldResemble, [][]float64{{23, 14}, {31, 18}})
			So(first, ShouldResemble, [][]float64{{-9, -10}, {43, 50}})
			So(left, ShouldResemble, [][]float64{{1, -2}, {3, 4}})
			So(right, ShouldResemble, [][]float64{{5, 6}, {7, 8}})
			So(node.Read(), ShouldResemble, second)
		})

		for index, operands := range [][2][][]float64{
			{{{1}}, {{1, 2}, {3, 4}}},
			{{{1, 2}, {3}}, {{1}, {2}}},
			{{{1, 2}}, {{1, 2}, {3}}},
		} {
			Convey(fmt.Sprintf("Incompatible shape %d is reported", index), func() {
				node := matrix.NewProduct(store.NewGet("left"), store.NewGet("right"))
				_, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"left": operands[0], "right": operands[1]}))
				So(errors.Is(err, core.ErrShape), ShouldBeTrue)
			})
		}

		Convey("A wrong operand type remains an explicit error", func() {
			node := matrix.NewProduct(store.NewGet("left"), store.NewGet("right"))
			_, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"left": [][]float64{{1}}, "right": "invalid"}))
			So(errors.Is(err, core.ErrWrongType), ShouldBeTrue)
		})
	})

	node := matrix.NewProduct(store.NewGet("left"), store.NewGet("right"))
	for range 3 {
		output := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{
			"left": [][]float64{{1, 2, 3}, {4, 5, 6}}, "right": [][]float64{{1, 0}, {0, 1}, {1, 1}},
		})))
		tests.Sound(t, node)
		got := output[0].([][]float64)
		for row, wanted := range [][]float64{{4, 5}, {10, 11}} {
			for column, value := range wanted {
				tests.EqualNumber(t, got[row][column], value)
			}
		}
	}
}

func BenchmarkProductNext(b *testing.B) {
	// One intercept plus the live resonance readout's 154 features.
	const dimension = 155
	left, right := make([][]float64, dimension), make([][]float64, dimension)

	for row := range left {
		left[row] = make([]float64, dimension)
		left[row][row] = 1
		right[row] = []float64{float64(row)}
	}
	input := tests.Record(map[string]any{"left": left, "right": right})
	node := matrix.NewProduct(store.NewGet("left"), store.NewGet("right"))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := transport.Evaluate[[][]float64](node, input); err != nil {
			b.Fatal(err)
		}
	}
}
