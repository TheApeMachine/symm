package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeetsExpectedEdgeGate(t *testing.T) {
	Convey("Given calibrated edge above spread cost", t, func() {
		measurements := []Measurement{
			{
				Source:          SourcePrediction,
				Symbol:          "BTC/USD",
				Price:           50_000,
				Strength:        0.7,
				Spread:          5,
				Confidence:      0.8,
				Surprise:        1,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.7,
				DecisionGrade:   DecisionGradeEdgeProvider,
				Category:        CategoryForecastEdge,
				Position:        PositionTypeLong,
			},
		}

		executionCost := ExecutionCostFromMarket(measurements, 0, 0, 0)

		Convey("It should allow entry", func() {
			So(MeetsExpectedEdgeGate(measurements, executionCost), ShouldBeTrue)
		})
	})

	Convey("Given strength without expected move", t, func() {
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

		executionCost := ExecutionCostFromMarket(measurements, 0, 0, 0)

		Convey("It should reject the strength proxy", func() {
			So(MeetsExpectedEdgeGate(measurements, executionCost), ShouldBeFalse)
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
				EdgeConfidence:  0.8,
				DecisionGrade:   DecisionGradeEdgeProvider,
				Category:        CategoryForecastEdge,
			},
		}

		executionCost := ExecutionCostFromMarket(measurements, 26, 0, 5)

		Convey("It should reject the entry", func() {
			So(MeetsExpectedEdgeGate(measurements, executionCost), ShouldBeFalse)
		})
	})
}

func TestBuildEntryCandidatesRequiresPositiveEdge(t *testing.T) {
	Convey("Given executable measurements without expected move", t, func() {
		measurements := []Measurement{
			{
				Source:        SourceHawkes,
				Symbol:        "BTC/USD",
				Strength:      0.8,
				Category:      CategoryFrenzy,
				Confidence:    0.8,
				Surprise:      1,
				DecisionGrade: DecisionGradeExecutable,
			},
		}

		executionCost := ExecutionCostFromMarket(measurements, 0, 0, 0)

		Convey("It should not build sizing candidates", func() {
			So(len(BuildEntryCandidates(measurements, executionCost)), ShouldEqual, 0)
		})
	})

	Convey("Given calibrated executable edge", t, func() {
		measurements := []Measurement{
			{
				Source:          SourceCVD,
				Symbol:          "BTC/USD",
				Strength:        0.8,
				Category:        CategoryAggressiveDrive,
				Confidence:      0.8,
				Surprise:        1,
				DecisionGrade:   DecisionGradeExecutable,
				ExpectedMoveBps: 80,
				EdgeConfidence:  0.7,
				Position:        PositionTypeLong,
			},
		}

		executionCost := ExecutionCostFromMarket(measurements, 0, 0, 0)
		candidates := BuildEntryCandidates(measurements, executionCost)

		Convey("It should build a long candidate", func() {
			So(len(candidates), ShouldEqual, 1)
			So(EntryCandidateLong(candidates[0]), ShouldBeTrue)
		})
	})
}

func TestAttributionFromConditionGroup(t *testing.T) {
	Convey("Given an exit branch with collapse and benign categories", t, func() {
		conditionGroup := &ConditionGroup{
			Boolean: BooleanTypeAnd,
			Conditions: []Condition{
				{
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Subject: Subject{
							Source: SourceExhaustion,
							Type:   SubjectCategory,
							Category: &Category{
								Type: CategoryMechanicalCollapse,
							},
						},
					},
				},
			},
		}

		measurements := []Measurement{
			{
				Source:   SourceLiquidity,
				Category: CategoryMedianDepth,
			},
			{
				Source:   SourceExhaustion,
				Category: CategoryMechanicalCollapse,
			},
		}

		attribution, ok := AttributionFromConditionGroup(conditionGroup, measurements, nil)

		Convey("It should prefer the matched exit category", func() {
			So(ok, ShouldBeTrue)
			So(attribution.Source, ShouldEqual, SourceExhaustion)
			So(attribution.Category, ShouldEqual, CategoryMechanicalCollapse)
		})
	})
}

func executableMeasurement(
	source SourceType,
	symbol string,
	category CategoryType,
	confidence float64,
	surprise float64,
	strength float64,
) Measurement {
	return Measurement{
		Source:          source,
		Symbol:          symbol,
		Price:           50_000,
		Strength:        strength,
		Volume:          1,
		Spread:          1,
		Elapsed:         1,
		Category:        category,
		Confidence:      confidence,
		Surprise:        surprise,
		ExpectedMoveBps: 80,
		EdgeConfidence:  confidence,
		Position:        PositionTypeLong,
		DecisionGrade:   DecisionGradeExecutable,
		ObservedAt:      time.Now().UTC(),
	}
}

func BenchmarkCandidateScore(b *testing.B) {
	candidate := EntryCandidate{
		Sources:    []SourceType{SourceCVD},
		Confidence: 0.7,
		EdgeBps:    40,
		CostBps:    20,
		Strength:   0.8,
		Novelty:    1.2,
	}
	anchors := CandidateAnchors{
		StrengthBySource: map[SourceType]float64{
			SourceCVD: 0.5,
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = candidateScore(candidate, anchors)
	}
}
