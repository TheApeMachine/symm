package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementEntryScore(t *testing.T) {
	Convey("Given a measurement confidence and its category baseline", t, func() {
		measurement := &Measurement{
			Confidence:    0.5,
			EntryBaseline: 0.25,
		}

		Convey("When Measurement.EntryScore normalizes the edge above baseline", func() {
			score, err := measurement.EntryScore()

			Convey("Then the score is the remaining-probability-normalized edge", func() {
				So(err, ShouldBeNil)
				So(score, ShouldAlmostEqual, (0.5-0.25)/(1-0.25), 1e-12)
			})
		})
	})

	Convey("Given a measurement with an invalid entry baseline", t, func() {
		measurement := &Measurement{
			Confidence:    0.5,
			EntryBaseline: 1,
		}

		Convey("When Measurement.EntryScore is requested", func() {
			_, err := measurement.EntryScore()

			Convey("Then it rejects the impossible baseline", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestMeasurementReady(t *testing.T) {
	Convey("Given a normalized known-category measurement", t, func() {
		measurement := NewMeasurement(SourceCVD, "BTC/USD", testMeasurementTime())
		err := measurement.ApplyClassifier(
			float64(CategoryIndex(CategoryAggressiveDrive)),
			0.8,
			0.4,
			0.3,
			1,
			map[string]float64{
				string(CategoryAggressiveDrive):   0.7,
				string(CategoryStochasticBalance): 0.3,
			},
		)

		Convey("When readiness is checked", func() {
			readyErr := measurement.Ready()

			Convey("Then it should accept the measurement", func() {
				So(err, ShouldBeNil)
				So(readyErr, ShouldBeNil)
			})
		})
	})

	Convey("Given an unnormalized distribution", t, func() {
		measurement := &Measurement{
			Source: SourceCVD,
			Symbol: "BTC/USD",
			At:     testMeasurementTime(),
			Distribution: map[CategoryType]float64{
				CategoryAggressiveDrive: 0.4,
			},
			Confidence:    0.8,
			EntryBaseline: 0.4,
			ExitBaseline:  0.3,
		}

		Convey("When readiness is checked", func() {
			err := measurement.Ready()

			Convey("Then it should reject the measurement", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "distribution must sum to one")
			})
		})
	})
}

func testMeasurementTime() time.Time {
	return time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
}
