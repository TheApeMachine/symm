package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestResonanceVerdict(t *testing.T) {
	Convey("Given a return head without a resolved target", t, func() {
		verdict := resonanceVerdict(nil)

		Convey("It should observe without pretending the adaptive estimator is unavailable", func() {
			So(verdict.Learning, ShouldEqual, "observing")
			So(verdict.LearningHealth, ShouldEqual, 0)
			So(verdict.Tuning, ShouldEqual, "recursive least squares")
			So(verdict.TuningHealth, ShouldEqual, 1)
			So(verdict.Direction, ShouldEqual, 0)
			So(verdict.Conviction, ShouldEqual, 0)
		})
	})

	Convey("Given a confidence-supported return forecast", t, func() {
		weakLong := resonanceVerdict(&types.ResonanceForecast{
			ExpectedReturn: 0.004,
			Confidence:     0.1,
		})
		short := resonanceVerdict(&types.ResonanceForecast{
			ExpectedReturn: -0.004,
			Confidence:     0.9,
		})

		Convey("It should predict immediately while keeping direction and probability separate", func() {
			So(weakLong.Learning, ShouldEqual, "predicting")
			So(weakLong.LearningHealth, ShouldEqual, 1)
			So(weakLong.Direction, ShouldEqual, 1)
			So(weakLong.Conviction, ShouldEqual, 0.1)
			So(short.Direction, ShouldEqual, -1)
			So(short.Conviction, ShouldEqual, 0.9)
		})
	})
}
