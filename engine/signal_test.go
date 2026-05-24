package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementTypeString(t *testing.T) {
	Convey("Given measurement types", t, func() {
		Convey("It should expose semantic labels", func() {
			So(Pump.String(), ShouldEqual, "pump")
			So(Basis.String(), ShouldEqual, "basis")
			So(MeasurementType(99).String(), ShouldEqual, "unknown")
		})
	})
}
