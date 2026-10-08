package algo_test

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
evaluateOLS drives one design through the OLS primitive.
*/
func evaluateOLS(node core.Primitive, design algo.Design) (algo.Fit, error) {
	var fit algo.Fit

	evaluation := transport.NewEvaluate(node)

	for out := range evaluation.Next(transport.NewValues(design).Next(nil)) {
		fit = *(*algo.Fit)(out)
	}

	return fit, evaluation.Error()
}

func TestOLSNext(t *testing.T) {
	Convey("Ordinary least squares recovers known coefficients", t, func() {
		node := algo.NewOLS(1e-15)
		random := rand.New(rand.NewSource(471))

		for trial := 0; trial < 35; trial++ {
			parameters := 1 + trial%4
			observations := parameters + 2 + trial%11
			x := make([][]float64, observations)
			y := make([]float64, observations)

			for row := range observations {
				x[row] = make([]float64, parameters)

				for column := range parameters {
					x[row][column] = random.NormFloat64()

					if column == 0 {
						x[row][column] = 1
					}

					y[row] += float64(column+1) * x[row][column]
				}

				y[row] += 0.2 * random.NormFloat64()
			}

			fit, err := evaluateOLS(node, algo.Design{X: x, Y: y})
			So(err, ShouldBeNil)
			So(fit.Defined, ShouldBeTrue)
			So(len(fit.Coefficients), ShouldEqual, parameters)

			for column := range parameters {
				So(fit.Coefficients[column], ShouldAlmostEqual, float64(column+1), 0.6)
			}
		}

		Convey("rank-deficient and empty designs are undefined, not fabricated", func() {
			for _, design := range []algo.Design{
				{X: [][]float64{{1, 1}, {1, 1}, {1, 1}}, Y: []float64{1, 2, 3}},
				{X: [][]float64{{1, 0}, {1, 1}}, Y: []float64{1, 2}},
				{X: [][]float64{}, Y: []float64{}},
			} {
				fit, err := evaluateOLS(node, design)
				So(err, ShouldBeNil)
				So(fit.Defined, ShouldBeFalse)
				So(len(fit.Coefficients), ShouldEqual, 0)
				So(math.IsNaN(fit.ResidualVariance), ShouldBeTrue)
			}
		})
	})
}
