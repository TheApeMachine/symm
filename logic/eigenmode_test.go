package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildEigenmodeScores(t *testing.T) {
	Convey("Given correlated momentum measurements", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourceCVD,
				"BTC/USD",
				1,
				1,
				1,
				1,
				1,
				CategoryAggressiveDrive,
				RegimeTypeNone,
				PositionTypeNone,
				0.92,
				1.2,
			),
			NewMeasurement(
				SourcePumpDump,
				"BTC/USD",
				1,
				1,
				1,
				1,
				1,
				CategoryVerticalIgnition,
				RegimeTypeNone,
				PositionTypeNone,
				0.88,
				1.1,
			),
			NewMeasurement(
				SourceDepthFlow,
				"BTC/USD",
				1,
				1,
				1,
				1,
				1,
				CategoryLoadedImbalance,
				RegimeTypeNone,
				PositionTypeNone,
				0.75,
				0.9,
			),
		}

		scores := BuildEigenmodeScores(measurements)

		Convey("It should produce a dominant momentum mode", func() {
			So(scores[EigenmodeMomentum], ShouldBeGreaterThan, 0.5)
		})
	})
}

func TestConditionGroupWeighted(t *testing.T) {
	Convey("Given a weighted playbook group", t, func() {
		thresholdCtx := NewThresholdContext(0.55, 0, 0)
		group := NewConditionGroup(BooleanTypeWeighted, []Condition{
			*NewCondition(
				ConditionIsGreaterThanOrEqual,
				ConditionOperand{Subject: *NewSubject(
					SourceCVD,
					SubjectConfidence,
					nil,
					nil,
					nil,
					0,
					0,
					0,
					0,
					0,
					0,
					0,
				)},
				ConditionOperand{Subject: *NewSubject(
					SourceNone,
					SubjectConfidence,
					nil,
					nil,
					nil,
					0,
					0,
					0,
					0,
					0,
					0.55,
					0,
				)},
			),
			*NewCondition(
				ConditionIsTrue,
				ConditionOperand{Subject: *NewSubject(
					SourceDepthFlow,
					SubjectCategory,
					&Category{Type: CategoryLoadedImbalance},
					nil,
					nil,
					0,
					0,
					0,
					0,
					0,
					0,
					0,
				)},
				ConditionOperand{},
			),
		})
		group.MinScore = 0.72
		group.Weights = []float64{0.6, 0.4}

		measurements := []Measurement{
			NewMeasurement(
				SourceCVD,
				"BTC/USD",
				1,
				1,
				1,
				1,
				1,
				CategoryAggressiveDrive,
				RegimeTypeNone,
				PositionTypeNone,
				0.54,
				1,
			),
			NewMeasurement(
				SourceDepthFlow,
				"BTC/USD",
				1,
				1,
				1,
				1,
				1,
				CategoryLoadedImbalance,
				RegimeTypeNone,
				PositionTypeNone,
				0.9,
				1,
			),
		}

		matched, _, err := group.EvaluateIndexed(measurements, nil, &thresholdCtx)

		Convey("It should pass when aggregate partial credit clears the bar", func() {
			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})
	})
}
