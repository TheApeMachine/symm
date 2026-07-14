package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObservationValidity(t *testing.T) {
	Convey("Given observation-window evidence counts", t, func() {
		Convey("Then empty windows are invalid", func() {
			validity := ObservationValidity(0)

			So(validity.State, ShouldEqual, ValidityInvalid)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldNotBeEmpty)
		})

		Convey("Then a single event stays provisional", func() {
			validity := ObservationValidity(1)

			So(validity.State, ShouldEqual, ValidityProvisional)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldNotBeEmpty)
		})

		Convey("Then corroborated windows are valid", func() {
			validity := ObservationValidity(2)

			So(validity.State, ShouldEqual, ValidityValid)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldBeEmpty)
		})
	})
}

func TestMeasurementValidateStruct(t *testing.T) {
	Convey("Given forward and backwards evidence intervals", t, func() {
		forward := Measurement{At: time.Unix(2, 0), ObservedFrom: time.Unix(1, 0)}
		backwards := Measurement{At: time.Unix(1, 0), ObservedFrom: time.Unix(2, 0)}

		Convey("Then only the forward interval is structurally valid", func() {
			So(forward.ValidateStruct(), ShouldBeNil)
			So(backwards.ValidateStruct(), ShouldNotBeNil)
		})
	})
}

func TestMeasurementInterval(t *testing.T) {
	Convey("Given explicit and implicit evidence provenance", t, func() {
		at := time.Unix(3, 0)
		explicit := Measurement{
			At: at, ObservedFrom: time.Unix(1, 0),
			Scale: ScaleReference{From: time.Unix(2, 0), Through: at},
		}
		implicit := Measurement{At: at}

		Convey("Then declared provenance wins before observation time", func() {
			from, through := explicit.Interval()
			implicitFrom, implicitThrough := implicit.Interval()
			So(from, ShouldEqual, explicit.ObservedFrom)
			So(through, ShouldEqual, at)
			So(implicitFrom, ShouldEqual, at)
			So(implicitThrough, ShouldEqual, at)
		})
	})
}
