package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestGaugeValue(t *testing.T) {
	convey.Convey("Given a measurement with live strength during SNR warmup", t, func() {
		measurement := Measurement{Strength: 2.4, SNR: 0}

		convey.Convey("GaugeValue should prefer Strength", func() {
			convey.So(GaugeValue(measurement), convey.ShouldEqual, 2.4)
		})
	})
}

func TestFinalizeMeasurementWarmup(t *testing.T) {
	convey.Convey("Given a cold score series", t, func() {
		ResetPlaybookScoreFloors()
		measurement := FinalizeMeasurement(Measurement{}, 1.8, "")

		convey.Convey("It should keep Strength while SNR stays zero until warmup", func() {
			convey.So(measurement.Strength, convey.ShouldEqual, 1.8)
			convey.So(measurement.SNR, convey.ShouldEqual, 0)
		})
	})
}
