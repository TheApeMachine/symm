package regulator

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/system"
)

func TestBuildPayload(t *testing.T) {
	Convey("Given a resolved posterior forecast and the controls it selected", t, func() {
		system.Cfg = system.NewConfig()
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		solver.optimizer.resolved = 3
		solver.history = []float64{0.2}
		result := optimizationResult{
			controls:      solver.optimizer.current,
			forecast:      learning.RLSOutput{Value: 0.01, Scale: 0.02, Ready: true},
			activity:      learning.RLSOutput{Value: 0.75, Scale: 0.1, Ready: true},
			forecastReady: true,
			activityReady: true,
			skill:         1.2,
			skillReady:    true,
			surprise:      0.2,
			energy:        0.3,
		}

		payload := solver.buildPayload(math.Log(1.01), result)

		Convey("It should report only the predictive model and controls it truly actuates", func() {
			So(payload.Status, ShouldEqual, "healthy")
			So(payload.PredictedReturn, ShouldAlmostEqual, math.Expm1(0.01))
			So(payload.PredictedActive, ShouldAlmostEqual, 0.75)
			So(payload.ActivityScale, ShouldAlmostEqual, 0.1)
			So(payload.Samples, ShouldEqual, 3)
			So(payload.Subsystems, ShouldHaveLength, 9)
			So(payload.Subsystems[0].Name, ShouldEqual, "model")
			So(payload.Subsystems[1].Name, ShouldEqual, "allocation")
			So(payload.Sparkline, ShouldResemble, []float64{0.2})
		})
	})
}

func TestFormatFloat(t *testing.T) {
	Convey("Given financial values with leading fractional zeroes", t, func() {
		Convey("It should preserve exact fixed-point presentation", func() {
			So(formatFloat(1.005, 3), ShouldEqual, "1.005")
			So(formatFloat(-0.25, 2), ShouldEqual, "-0.25")
		})
	})
}
