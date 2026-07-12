package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementTyped(t *testing.T) {
	Convey("Given a numerical measurement identity", t, func() {
		measurement := Measurement{Metric: MetricConditionalIntensity}

		Convey("It should identify the typed contract", func() {
			So(measurement.Typed(), ShouldBeTrue)
		})
	})

	Convey("Given an unmigrated metric-map measurement", t, func() {
		measurement := Measurement{Metrics: map[string]float64{"strength": 1}}

		Convey("It should identify the compatibility contract", func() {
			So(measurement.Typed(), ShouldBeFalse)
		})
	})
}
