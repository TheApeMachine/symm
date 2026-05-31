package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAggregateThesisScore(t *testing.T) {
	Convey("Given normalized category SNRs", t, func() {
		measurements := []Measurement{
			{Category: CategoryAggressiveDrive, SNR: 2.0},
			{Category: CategoryAggressiveDrive, SNR: 4.0},
		}
		relevant := map[CategoryType]bool{CategoryAggressiveDrive: true}

		Convey("It should return RMS sigma units", func() {
			score := AggregateThesisScore(measurements, relevant)

			So(score, ShouldBeGreaterThan, 3.0)
			So(score, ShouldBeLessThan, 3.2)
		})
	})
}

func TestRequiredThesisScore(t *testing.T) {
	Convey("Given fees spread and edge multiple", t, func() {
		required := RequiredThesisScore(1.5, 0.4, 12)

		Convey("It should scale friction into thesis sigma units", func() {
			So(required, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given separate entry and exit fees", t, func() {
		makerTaker := RequiredThesisScoreForFees(3, 0.25, 0.40, 0)
		takerTaker := RequiredThesisScoreForFees(3, 0.40, 0.40, 0)

		Convey("It should price the actual round trip", func() {
			So(makerTaker, ShouldAlmostEqual, 1.95)
			So(takerTaker, ShouldAlmostEqual, 2.4)
			So(makerTaker, ShouldBeLessThan, takerTaker)
		})
	})
}
