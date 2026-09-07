package rls_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation/rls"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestUpdateNext(t *testing.T) {
	Convey("Given a signed rank-one observation with forgetting", t, func() {
		prior := [][]float64{{1, 0}, {0, 1}}
		for _, target := range []float64{-3, 3} {
			output, err := transport.Evaluate[map[string]core.Primitive](rls.NewUpdate(), core.Record(map[string]any{
				"beta": []float64{0, 0}, "root": prior, "factor": []float64{1, 2},
				"lambda": .98, "target": target, "prediction": 0.0, "noise_shape": 0.0, "noise_scale": 0.0,
			}))
			So(err, ShouldBeNil)
			beta := core.To[[]float64](output["beta"])
			So(beta[0], ShouldAlmostEqual, target/5.98)
			So(beta[1], ShouldAlmostEqual, 2*target/5.98)
			root := core.To[[][]float64](output["root"])
			for row := range root {
				for column := range root {
					covariance := root[row][0]*root[column][0] + root[row][1]*root[column][1]
					identity := 0.0
					if row == column {
						identity = 1
					}
					So(covariance, ShouldAlmostEqual, (identity-float64((row+1)*(column+1))/5.98)/.98)
				}
			}
			So(tests.Number(t, output, "noise_scale"), ShouldAlmostEqual, .5*target*target/5.98)
			So(prior, ShouldResemble, [][]float64{{1, 0}, {0, 1}})
		}
	})
}
