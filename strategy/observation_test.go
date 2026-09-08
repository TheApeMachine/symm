package strategy

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"testing"
)

func TestMeasurementFromObservation(t *testing.T) {
	Convey("A historical observation projects only the facts it defines", t, func() {
		measurement := measurementFromObservation(rehearsalObservation(1, 0, "BTC/USD", 99, 101, 100))

		So(measurement.Source, ShouldEqual, rawMarketSource)
		So(measurement.Label, ShouldEqual, "BTC/USD")
		So(measurement.Metrics["bid"].Raw, ShouldEqual, 99)
		So(measurement.Metrics["ask"].Raw, ShouldEqual, 101)
		So(measurement.Metrics["last"].Raw, ShouldEqual, 100)
		So(measurement.Metrics["spread"].Raw, ShouldBeGreaterThan, 0)
		So(measurement.Metrics["depth"].Raw, ShouldEqual, 0)

		space := grid.NewSpace()
		So(space.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
		So(measurement.Err, ShouldBeNil)
		So(space.Version, ShouldEqual, 1)
	})
}
