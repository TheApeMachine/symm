package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConditionEvaluate(t *testing.T) {
	Convey("Given Hawkes frenzy with surprise 2.5", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryFrenzy,
				RegimeTypeNone,
				PositionTypeNone,
				0.8,
				2.5,
			),
		}

		Convey("ConditionIsTrue should pass for matching category", func() {
			condition := NewCondition(
				ConditionIsTrue,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
					SubjectCategory,
					NewCategory(CategoryFrenzy),
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
			)

			matched, err := condition.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})

		Convey("ConditionIsFalse should pass when category does not match", func() {
			condition := NewCondition(
				ConditionIsFalse,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
					SubjectCategory,
					NewCategory(CategorySaturation),
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
			)

			matched, err := condition.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})

		Convey("ConditionIsGreaterThanOrEqual should compare live confidence to threshold", func() {
			condition := NewCondition(
				ConditionIsGreaterThanOrEqual,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
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
					0.50,
					0,
				)},
			)

			matched, err := condition.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})

		Convey("ConditionIsGreaterThan should compare live surprise to threshold", func() {
			condition := NewCondition(
				ConditionIsGreaterThan,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
					SubjectSurprise,
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
					SubjectSurprise,
					nil,
					nil,
					nil,
					0,
					0,
					0,
					0,
					0,
					0,
					2.0,
				)},
			)

			matched, err := condition.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})

		Convey("ConditionIsWithin should use right spread as tolerance", func() {
			condition := NewCondition(
				ConditionIsWithin,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
					SubjectSurprise,
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
					SubjectSurprise,
					nil,
					nil,
					nil,
					0,
					0,
					0.6,
					0,
					0,
					0,
					2.4,
				)},
			)

			matched, err := condition.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})
	})

	Convey("Given measurements from two sources", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryFrenzy,
				RegimeTypeNone,
				PositionTypeNone,
				0.9,
				2.5,
			),
			NewMeasurement(
				SourceCausal,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryCausalNoise,
				RegimeTypeNone,
				PositionTypeNone,
				0.4,
				1.0,
			),
		}

		Convey("It should compare confidence across sources", func() {
			condition := NewCondition(
				ConditionIsGreaterThan,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
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
					SourceCausal,
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
			)

			matched, err := condition.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})
	})

	Convey("Given measurements from two sources without a source filter", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryFrenzy,
				RegimeTypeNone,
				PositionTypeNone,
				0.9,
				2.5,
			),
			NewMeasurement(
				SourceToxicity,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategorySaturation,
				RegimeTypeNone,
				PositionTypeNone,
				0.4,
				1.0,
			),
		}

		Convey("ConditionIsFalse should scan every measurement before passing", func() {
			condition := NewCondition(
				ConditionIsFalse,
				ConditionOperand{Subject: *NewSubject(
					SourceNone,
					SubjectCategory,
					NewCategory(CategoryFrenzy),
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
			)

			matched, err := condition.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeFalse)
		})
	})
}

func TestConditionGroupEvaluate(t *testing.T) {
	Convey("Given an AND group over two source conditions", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryFrenzy,
				RegimeTypeNone,
				PositionTypeNone,
				0.8,
				2.5,
			),
			NewMeasurement(
				SourceToxicity,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryToxicBluff,
				RegimeTypeNone,
				PositionTypeNone,
				0.7,
				1.5,
			),
		}

		group := NewConditionGroup(BooleanTypeAnd, []Condition{
			*NewCondition(
				ConditionIsTrue,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
					SubjectCategory,
					NewCategory(CategoryFrenzy),
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
			*NewCondition(
				ConditionIsGreaterThan,
				ConditionOperand{Subject: *NewSubject(
					SourceToxicity,
					SubjectSurprise,
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
					SubjectSurprise,
					nil,
					nil,
					nil,
					0,
					0,
					0,
					0,
					0,
					0,
					1.0,
				)},
			),
		})

		Convey("It should pass when every condition passes", func() {
			matched, err := group.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})

		Convey("It should fail when one condition fails", func() {
			measurements[1].Surprise = 0.5

			matched, err := group.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeFalse)
		})
	})
}

func TestConditionGroupOrEarliestAnchor(t *testing.T) {
	Convey("Given an OR group with matches at different timeline indices", t, func() {
		group := NewConditionGroup(BooleanTypeOr, []Condition{
			*NewCondition(
				ConditionIsTrue,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
					SubjectCategory,
					NewCategory(CategoryFrenzy),
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
			*NewCondition(
				ConditionIsTrue,
				ConditionOperand{Subject: *NewSubject(
					SourceCausal,
					SubjectCategory,
					NewCategory(CategoryEndogenousAlpha),
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

		measurements := []Measurement{
			NewMeasurement(
				SourceCausal,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryEndogenousAlpha,
				RegimeTypeNone,
				PositionTypeNone,
				0,
				0,
			),
			NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryFrenzy,
				RegimeTypeNone,
				PositionTypeNone,
				0,
				0,
			),
		}

		matched, matchIndex, err := group.EvaluateIndexed(measurements, nil, nil)

		Convey("It should anchor on the earliest matching index", func() {
			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
			So(matchIndex, ShouldEqual, 0)
		})
	})
}
