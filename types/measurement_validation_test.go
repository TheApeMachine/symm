package types

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementValidate(t *testing.T) {
	Convey("Given a complete numerical measurement", t, func() {
		measurement := validMeasurement()

		Convey("When its contract is validated", func() {
			err := measurement.Validate()

			Convey("Then logic can consume it without source-specific knowledge", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When normalized zero is explicitly available", func() {
			measurement.Normalized = OptionalValue{Value: 0, Available: true}

			Convey("Then it remains distinct from a missing normalized value", func() {
				So(measurement.Validate(), ShouldBeNil)
				So(measurement.Normalized.Available, ShouldBeTrue)
			})
		})

		Convey("When the raw value is non-finite", func() {
			measurement.Raw = math.Inf(1)

			Convey("Then the corrupted estimate is rejected", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When the horizon does not describe its timestamps", func() {
			measurement.Horizon += time.Second

			Convey("Then temporal meaning cannot silently drift", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When a provisional estimate omits its limitation", func() {
			measurement.Validity.State = ValidityProvisional

			Convey("Then provisional data cannot appear fully explained", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When scale construction includes future data", func() {
			measurement.Scale.Through = measurement.At.Add(time.Second)

			Convey("Then look-ahead evidence is rejected", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When typed evidence also contains a mutable legacy metric map", func() {
			measurement.Metrics = map[string]float64{"price": 100}

			Convey("Then the immutable numerical boundary is rejected", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When typed evidence also contains a signal-owned category", func() {
			measurement.Categories = []Category{{Type: OrganicTrend}}

			Convey("Then interpretation cannot enter the measurement epoch", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})

		Convey("When unavailable uncertainty still carries interval values", func() {
			measurement.Uncertainty.Lower = 1

			Convey("Then stale uncertainty cannot appear absent", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})
	})

	Convey("Given a legacy category measurement", t, func() {
		measurement := Measurement{Source: SourceCVD, Symbol: "BTC/USD"}

		Convey("When the numerical contract is requested", func() {
			Convey("Then the migration boundary stays explicit", func() {
				So(measurement.Validate(), ShouldNotBeNil)
			})
		})
	})
}

func validMeasurement() Measurement {
	at := time.Date(2026, 7, 12, 3, 0, 1, 0, time.UTC)
	from := at.Add(-time.Second)

	return Measurement{
		Source:       SourceHawkes,
		Metric:       MetricConditionalIntensity,
		Subject:      SubjectHawkesProcess,
		Stream:       "trades",
		Symbol:       "BTC/USD",
		Side:         SideBuy,
		At:           at,
		ObservedFrom: from,
		Horizon:      at.Sub(from),
		Unit:         UnitEventsPerSecond,
		Raw:          3,
		Maturity:     0.5,
		Validity: MeasurementValidity{
			State:     ValidityValid,
			Readiness: ReadinessIntensity,
		},
		Scale: ScaleReference{
			Kind:    ScaleObservationWindow,
			From:    from,
			Through: at,
		},
	}
}
