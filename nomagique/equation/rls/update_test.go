package rls_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation/rls"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestUpdateNext(t *testing.T) {
	Convey("Given a signed rank-one observation with forgetting", t, func() {
		prior := [][]float64{{1, 0}, {0, 1}}

		for _, target := range []float64{-3, 3} {
			output, err := transport.Evaluate(rls.NewUpdate(), transport.Values(rls.Observation{
				Forecast: rls.Forecast{
					State: rls.State{
						Beta: []float64{0, 0}, Root: prior,
					},
					Factor:     []float64{1, 2},
					Prediction: 0,
				},
				Lambda: 0.98, Target: target,
			}))
			So(err, ShouldBeNil)
			So(output.Beta[0], ShouldAlmostEqual, target/5.98)
			So(output.Beta[1], ShouldAlmostEqual, 2*target/5.98)

			for row := range output.Root {
				for column := range output.Root {
					covariance := output.Root[row][0]*output.Root[column][0] + output.Root[row][1]*output.Root[column][1]
					identity := 0.0

					if row == column {
						identity = 1
					}

					So(covariance, ShouldAlmostEqual, (identity-float64((row+1)*(column+1))/5.98)/.98)
				}
			}

			So(output.NoiseScale, ShouldAlmostEqual, .5*target*target/5.98)
			So(prior, ShouldResemble, [][]float64{{1, 0}, {0, 1}})
		}
	})
}
