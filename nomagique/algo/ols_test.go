package algo_test

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestOLSNext(t *testing.T) {
	Convey("Ordinary least squares matches an independent LU normal-equations fit", t, func() {
		node := algo.NewOLS(1e-15)
		random := rand.New(rand.NewSource(471))

		for trial := 0; trial < 35; trial++ {
			parameters := 1 + trial%4
			observations := parameters + 2 + trial%11
			x := make([][]float64, observations)
			flat := make([]float64, 0, observations*parameters)
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
				flat = append(flat, x[row]...)
			}

			expected := statistic.FitOLS(flat, y, parameters)
			fit, err := transport.Evaluate(node, transport.Values(algo.Design{X: x, Y: y}))
			So(err, ShouldBeNil)
			So(fit.Defined, ShouldEqual, expected.Defined)
			So(len(fit.Coefficients), ShouldEqual, parameters)
			So(len(fit.CoefficientVariance), ShouldEqual, parameters)

			for index := range parameters {
				So(fit.Coefficients[index], ShouldAlmostEqual, expected.Coefficients[index])
				So(fit.CoefficientVariance[index], ShouldAlmostEqual, expected.CoefficientVariance[index])
			}

			So(fit.ResidualSSE, ShouldAlmostEqual, expected.ResidualSSE)
			So(fit.ResidualVariance, ShouldAlmostEqual, expected.ResidualVariance)
		}

		for _, design := range []algo.Design{
			{X: [][]float64{{1, 1}, {1, 1}, {1, 1}}, Y: []float64{1, 2, 3}},
			{X: [][]float64{{1, 0}, {1, 1}}, Y: []float64{1, 2}},
			{X: [][]float64{}, Y: []float64{}},
		} {
			fit, err := transport.Evaluate(node, transport.Values(design))
			So(err, ShouldBeNil)
			So(fit.Defined, ShouldBeFalse)
			So(len(fit.Coefficients), ShouldEqual, 0)
			So(math.IsNaN(fit.ResidualVariance), ShouldBeTrue)
		}
	})
}
