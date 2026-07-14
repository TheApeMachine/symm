package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObservationMeasurement(testingTB *testing.T) {
	Convey("Given one observation-layer measurement request", testingTB, func() {
		at := time.Unix(5, 0)

		measurement := ObservationMeasurement(
			SourcePumpDump, PumpDump, MetricRVOL,
			SubjectPumpVolumeLift, "BTC/USD", at,
			UnitDimensionless, 1.25, 0.5,
		)

		Convey("Then it should retain the shared validity and scale contract", func() {
			So(measurement.Subject, ShouldEqual, SubjectPumpVolumeLift)
			So(measurement.Maturity, ShouldEqual, 0.5)
			So(measurement.Normalized, ShouldNotBeNil)
			So(*measurement.Normalized, ShouldEqual, 1.25)
			So(measurement.Validity.State, ShouldEqual, ValidityValid)
			So(measurement.Validity.Readiness, ShouldEqual, ReadinessObservation)
			So(measurement.Scale.Kind, ShouldEqual, ScaleObservationWindow)
			So(measurement.Scale.From, ShouldEqual, at)
		})
	})
}

func TestObservationSideMeasurement(testingTB *testing.T) {
	Convey("Given one directional observation measurement", testingTB, func() {
		at := time.Unix(6, 0)

		measurement := ObservationSideMeasurement(
			SourceToxicity, Toxicity, MetricFillVolume,
			SubjectLevel3Tape, "ETH/USD", SideBuy, at,
			UnitQuoteCurrency, 10, 0.66,
		)

		Convey("Then it should preserve side semantics", func() {
			So(measurement.Side, ShouldEqual, SideBuy)
			So(measurement.Subject, ShouldEqual, SubjectLevel3Tape)
		})
	})
}
