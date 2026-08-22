package regulator

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/system"
)

func TestBuildPayload(t *testing.T) {
	Convey("Given a resolved posterior forecast and the controls it selected", t, func() {
		system.Cfg = system.NewConfig()
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		solver.optimizer.resolved = 3
		solver.history = []float64{0.2}
		solver.markSamples = 12
		solver.lastMarkContext = markContext{samples: 4}
		solver.lastMarkSymbol = "BTC/USD"
		solver.lastMarkReturn = math.Log(1.01)
		solver.lastMarkDrawdown = math.Log(0.98)
		solver.lastMarkFloor = 0.015
		solver.lastMarkSurge = true
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
			So(payload.MarkSamples, ShouldEqual, uint64(12))
			So(payload.IntervalMarks, ShouldEqual, 4)
			So(payload.LastMarkFloor, ShouldAlmostEqual, 0.015)
			So(payload.LastMarkSurge, ShouldBeTrue)
			So(payload.Subsystems, ShouldHaveLength, 10)
			So(payload.Subsystems[0].Name, ShouldEqual, "model")
			So(payload.Subsystems[1].Name, ShouldEqual, "allocation")
			So(payload.Subsystems[2].Name, ShouldEqual, "thesis_score")
			So(payload.Subsystems[3].Name, ShouldEqual, "confidence")
			So(payload.Subsystems[4].Name, ShouldEqual, "support")
			So(payload.Subsystems[5].Name, ShouldEqual, "contradiction")
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
