package algo_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestRLS(t *testing.T) {
	Convey("Given RLS Prediction and Update closures", t, func() {
		predict := algo.NewRLSPrediction()
		update := algo.NewRLSUpdate()

		// 2-dimensional system: y = 2*x0 + 3*x1
		state := algo.RLSState{
			Beta:   []float64{0.0, 0.0},
			Design: []float64{core.Unit, 2.0},
			Root: [][]float64{
				{core.Unit, 0.0},
				{0.0, core.Unit},
			},
			NoiseShape:   core.Unit,
			NoiseScale:   core.Unit,
			Observations: core.Unit,
		}

		Convey("When predicting prior to update", func() {
			forecast := predict(state)
			So(forecast.Prediction, ShouldEqual, 0.0) // initial beta is 0
			So(forecast.Ready, ShouldBeTrue)
			So(forecast.PredictiveVariance, ShouldBeGreaterThan, 0.0)

			Convey("When updating with observation target y = 8.0", func() {
				obs := algo.RLSObservation{
					RLSForecast: forecast,
					Lambda:      0.99,
					Target:      8.0,
				}

				posterior := update(obs)
				So(posterior.Innovation, ShouldEqual, 8.0)
				So(posterior.Beta[0], ShouldBeGreaterThan, 0.0)
				So(posterior.Beta[1], ShouldBeGreaterThan, 0.0)

				Convey("When predicting next step with updated posterior", func() {
					nextState := posterior.RLSState
					nextState.Design = []float64{core.Unit, 2.0}
					nextForecast := predict(nextState)
					So(nextForecast.Prediction, ShouldBeGreaterThan, 0.0)
				})
			})
		})
	})
}
