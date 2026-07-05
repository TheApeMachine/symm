package logic

import (
	"testing"

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
