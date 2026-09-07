package matrix_test

import (
	"errors"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestIdentityNext(t *testing.T) {
	Convey("Given dimensions supplied at evaluation time", t, func() {
		Convey("An empty identity stays empty and earlier matrices remain immutable", func() {
			node := matrix.NewIdentity()
			first, err := transport.Evaluate[[][]float64](node, core.From(2.0))
			So(err, ShouldBeNil)
			So(first, ShouldResemble, [][]float64{{1, 0}, {0, 1}})
			empty, err := transport.Evaluate[[][]float64](node, core.From(0.0))
			So(err, ShouldBeNil)
			So(empty, ShouldBeEmpty)
			So(first, ShouldResemble, [][]float64{{1, 0}, {0, 1}})
		})

		for _, size := range []float64{-1, 1.5} {
			Convey(fmt.Sprintf("Invalid dimension %g is rejected by Range", size), func() {
				_, err := transport.Evaluate[[][]float64](matrix.NewIdentity(), core.From(size))
				So(errors.Is(err, core.ErrShape), ShouldBeTrue)
			})
		}
	})

	node := matrix.NewIdentity()
	for _, size := range []int{1, 3, 2} {
		output := tests.Drain(t, node, tests.Values(float64(size)))
		tests.Sound(t, node)
		got := output[0].([][]float64)
		if len(got) != size {
			t.Fatal(got)
		}
		for row := range size {
			for column := range size {
				expected := 0.0
				if row == column {
					expected = 1
				}
				tests.EqualNumber(t, got[row][column], expected)
			}
		}
	}
}
