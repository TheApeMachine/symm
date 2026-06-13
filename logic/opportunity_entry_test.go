package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQualifiesForOpportunityEntry(t *testing.T) {
	thresholdCtx := NewThresholdContext(0.55, 0, 0)

	Convey("Given a high-confidence pump measurement", t, func() {
		measurements := []Measurement{
			{
				Source:          SourcePrediction,
				Symbol:          "CELO/USD",
				Price:           0.25,
				Strength:        0.8,
				Volume:          100,
				Spread:          0.00003,
				Elapsed:         1,
				Category:        CategoryForecastEdge,
				Confidence:      0.8,
				Surprise:        1,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.7,
				Position:        PositionTypeLong,
				DecisionGrade:   DecisionGradeEdgeProvider,
			},
			{
				Source:          SourcePumpDump,
				Symbol:          "CELO/USD",
				Price:           0.25,
				Strength:        1.2,
				Volume:          100,
				Spread:          0.00003,
				Elapsed:         1,
				Category:        CategoryVerticalIgnition,
				Confidence:      0.9,
				Surprise:        1.4,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.9,
				Position:        PositionTypeLong,
				DecisionGrade:   DecisionGradeExecutable,
			},
		}

		Convey("It should qualify for an opportunity slot", func() {
			So(QualifiesForOpportunityEntry(measurements, thresholdCtx), ShouldBeTrue)
		})
	})

	Convey("Given elevated confidence without surprise", t, func() {
		measurements := []Measurement{
			{
				Source:          SourcePumpDump,
				Symbol:          "CELO/USD",
				Price:           0.25,
				Strength:        1.2,
				Volume:          100,
				Spread:          0.00003,
				Elapsed:         1,
				Category:        CategoryVerticalIgnition,
				Confidence:      0.9,
				Surprise:        0.5,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.9,
				Position:        PositionTypeLong,
				DecisionGrade:   DecisionGradeExecutable,
			},
		}

		Convey("It should not qualify", func() {
			So(QualifiesForOpportunityEntry(measurements, thresholdCtx), ShouldBeFalse)
		})
	})

	Convey("Given surprise without positive edge", t, func() {
		measurements := []Measurement{
			{
				Source:        SourceHawkes,
				Symbol:        "BTC/USD",
				Price:         50_000,
				Strength:      0.4,
				Volume:        100,
				Spread:        0.01,
				Elapsed:       1,
				Category:      CategoryLaminar,
				Confidence:    0.9,
				Surprise:      1.5,
				Position:      PositionTypeLong,
				DecisionGrade: DecisionGradeExecutable,
			},
		}

		Convey("It should not qualify", func() {
			So(QualifiesForOpportunityEntry(measurements, thresholdCtx), ShouldBeFalse)
		})
	})
}
