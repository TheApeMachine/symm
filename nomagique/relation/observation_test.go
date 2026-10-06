package relation

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMeasurementAppend(t *testing.T) {
	Convey("Given a finalized measurement and the coordinates it projects", t, func() {
		measurement := data.NewMeasurement(1, "TEST/USD", "cvd", 1, 1)
		measurement.At = time.Unix(10, 0).UTC()
		measurement.From = time.Unix(9, 0).UTC()
		measurement.Write(
			data.NewMetric("signed_net_fraction:buy", 0.25, data.UnitRatio, data.TimescaleTick),
		)

		coordinates := []Coordinate{{
			Metric: "signed_net_fraction", Side: "buy",
			Unit: data.UnitRatio, Timescale: data.TimescaleTick,
		}}

		observations, err := splitMeasurement(measurement, coordinates, 7)

		Convey("It stamps identity from the measurement and reads the metric by key", func() {
			if err != nil {
				t.Logf("split error: %v", err)
			}

			So(err, ShouldBeNil)
			So(observations, ShouldHaveLength, 1)
			So(observations[0].Raw, ShouldEqual, 0.25)
			So(observations[0].Coordinate.Symbol, ShouldEqual, "TEST/USD")
			So(observations[0].Coordinate.Source, ShouldEqual, "cvd")
			So(observations[0].Coordinate.Epoch, ShouldEqual, 7)
			So(observations[0].MeasurementID, ShouldEqual, measurement.ID)
		})
	})
}
