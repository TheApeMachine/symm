package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeetsExpectedEdgeGate(t *testing.T) {
	Convey("Given a strength proxy above spread cost", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				50_000,
				0.01,
				1,
				5,
				1,
				CategoryFrenzy,
				RegimeTypeNone,
				PositionTypeNone,
				0.8,
				1,
			),
		}

		Convey("It should allow entry without prediction", func() {
			So(MeetsExpectedEdgeGate(measurements, 0, 0, 0), ShouldBeTrue)
		})
	})

	Convey("Given a prediction below costs", t, func() {
		measurements := []Measurement{
			{
				Source:          SourcePrediction,
				Symbol:          "BTC/USD",
				Price:           50_000,
				Strength:        0.00001,
				ExpectedMoveBps: 1,
				Spread:          20,
				Confidence:      0.8,
				Surprise:        1,
			},
		}

		Convey("It should reject the entry", func() {
			So(MeetsExpectedEdgeGate(measurements, 26, 0, 5), ShouldBeFalse)
		})
	})
}

func TestExitTierForCategory(t *testing.T) {
	Convey("Given mechanical collapse", t, func() {
		Convey("It should require full liquidation", func() {
			So(ExitFractionForTier(ExitTierForCategory(CategoryMechanicalCollapse), 0), ShouldEqual, 1)
		})
	})

	Convey("Given Hawkes saturation", t, func() {
		Convey("It should reduce by half", func() {
			So(ExitFractionForTier(ExitTierForCategory(CategorySaturation), 0), ShouldEqual, 0.5)
		})
	})
}
