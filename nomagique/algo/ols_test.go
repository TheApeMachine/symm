package algo_test

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestOLSNext(t *testing.T) {
	Convey("Ordinary least squares recovers known coefficients", t, func() {
		node := algo.NewOLS()
		random := rand.New(rand.NewSource(471))

		for trial := 0; trial < 35; trial++ {
			parameters := 1 + trial%4
			observations := parameters + 2 + trial%11
			rows := make([][]float64, observations)

			for row := range observations {
				rows[row] = make([]float64, parameters+1)

				for column := range parameters {
					rows[row][column] = random.NormFloat64()

					if column == 0 {
						rows[row][column] = 1
					}

					rows[row][parameters] += float64(column+1) * rows[row][column]
				}

				rows[row][parameters] += 0.2 * random.NormFloat64()
			}

			out := tests.CollectSeq[[]float64](node.Next(data.NewValue(rows)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			fit := out[0]
			So(fit[0], ShouldEqual, 1)
			So(fit[3], ShouldEqual, parameters)
			So(len(fit), ShouldBeGreaterThanOrEqualTo, 7+parameters)

			for column := range parameters {
				So(fit[7+column], ShouldAlmostEqual, float64(column+1), 0.6)
			}
		}

		Convey("rank-deficient and empty designs are undefined, not fabricated", func() {
			for _, rows := range [][][]float64{
				{{1, 1, 1}, {1, 1, 2}, {1, 1, 3}},
				{{1, 0, 1}, {1, 1, 2}},
				{},
			} {
				out := tests.CollectSeq[[]float64](node.Next(data.NewValue(rows)))
				So(node.Error(), ShouldBeNil)
				So(len(out), ShouldEqual, 1)
				So(out[0][0], ShouldEqual, 0)
				So(len(out[0]), ShouldEqual, 7)
				So(math.IsNaN(out[0][5]), ShouldBeTrue)
			}
		})

		Convey("ragged observation rows are a shape error", func() {
			errNode := algo.NewOLS()

			for range errNode.Next(data.NewValue([][]float64{{1, 2, 3}, {1, 2}})) {
				t.Fatal("a ragged design must yield nothing")
			}

			So(errNode.Error(), ShouldNotBeNil)
		})
	})
}
