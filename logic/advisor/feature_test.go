package advisor

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMetricPrediction(t *testing.T) {
	Convey("Given opposing moves for one adaptive metric", t, func() {
		prediction := NewMetricPrediction("pumpdump/notional_rate_velocity", INCREASE, DECREASE)

		Convey("it declares raw zero-threshold support and contradiction events", func() {
			So(prediction.Support.Label, ShouldEqual, "pumpdump/notional_rate_velocity")
			So(prediction.Support.Type, ShouldEqual, METRIC)
			So(prediction.Support.Move, ShouldEqual, INCREASE)
			So(prediction.Support.Value, ShouldEqual, 0.0)
			So(prediction.Support.Unit, ShouldEqual, RAW)
			So(prediction.Contradict.Label, ShouldEqual, prediction.Support.Label)
			So(prediction.Contradict.Move, ShouldEqual, DECREASE)
		})
	})
}
