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

func TestFinalizeSNR(t *testing.T) {
	convey.Convey("Given a scorer that is still warming up", t, func() {
		measurement := FinalizeSNR(Measurement{}, 1.8, func(float64) float64 { return 0 })

		convey.Convey("It should keep Strength while SNR stays at raw fallback", func() {
			convey.So(measurement.Strength, convey.ShouldEqual, 1.8)
			convey.So(measurement.SNR, convey.ShouldEqual, 1.8)
		})
	})
}
