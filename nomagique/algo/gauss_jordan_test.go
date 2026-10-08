package algo_test

import (
	"errors"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestGaussJordanNext(t *testing.T) {
	Convey("Known solutions and independently multiplied inverses survive pivoting", t, func() {
		node := algo.NewGaussJordan(1e-15)
		random := rand.New(rand.NewSource(192))

		for trial := 0; trial < 30; trial++ {
			size := 1 + trial%5
			left := make([][]float64, size)
			right := make([][]float64, size)
			expected := make([]float64, size)

			for row := range size {
				expected[row] = random.NormFloat64()
				left[row] = make([]float64, size)
				right[row] = make([]float64, 1+size)

				for column := range size {
					left[row][column] = random.NormFloat64()
				}

				left[row][row] += float64(size) + 2
			}

			for row := range size {
				for column := range size {
					right[row][0] += left[row][column] * expected[column]
				}
			}

			if trial%2 == 0 && size > 1 {
				left[0], left[size-1] = left[size-1], left[0]
				right[0], right[size-1] = right[size-1], right[0]
			}

			for row := range size {
				right[row][row+1] = 1
			}

			out := tests.CollectSeq[algo.Solution](node.Next(transport.NewValues(algo.System{Left: left, Right: right}).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			solution := out[0]
			So(solution.Defined, ShouldBeTrue)

			for row := range size {
				So(solution.Solution[row][0], ShouldAlmostEqual, expected[row])
			}

			for row := range size {
				for column := range size {
					product := 0.0

					for inner := range size {
						product += left[row][inner] * solution.Solution[inner][column+1]
					}

					target := 0.0

					if row == column {
						target = 1
					}

					So(product, ShouldAlmostEqual, target)
				}
			}
		}

		Convey("A singular system is undefined, and a later solve is independent", func() {
			outSingular := tests.CollectSeq[algo.Solution](node.Next(transport.NewValues(algo.System{
				Left:  [][]float64{{1, 2}, {2, 4}},
				Right: [][]float64{{1}, {2}},
			}).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(outSingular[0].Defined, ShouldBeFalse)
			So(len(outSingular[0].Solution), ShouldEqual, 0)

			outSol := tests.CollectSeq[algo.Solution](node.Next(transport.NewValues(algo.System{
				Left:  [][]float64{{2, 0}, {0, 3}},
				Right: [][]float64{{4}, {9}},
			}).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(outSol[0].Defined, ShouldBeTrue)
			So(outSol[0].Solution[0][0], ShouldEqual, 2)
			So(outSol[0].Solution[1][0], ShouldEqual, 3)
		})

		Convey("A non-square system is a shape error", func() {
			errNode := algo.NewGaussJordan(1e-15)
			_ = tests.CollectSeq[algo.Solution](errNode.Next(transport.NewValues(algo.System{
				Left:  [][]float64{{1, 2, 3}, {4, 5, 6}},
				Right: [][]float64{{1}, {2}},
			}).Next(nil)))
			So(errors.Is(errNode.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
