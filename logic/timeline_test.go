package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestBranchEvaluateTimelineSlicing(t *testing.T) {
	Convey("Given a parent branch keyed on coiled compression", t, func() {
		parent := &Branch{
			ConditionGroup: NewConditionGroup(BooleanTypeAnd, []Condition{
				*NewCondition(
					ConditionIsTrue,
					ConditionOperand{Subject: *NewSubject(
						SourcePumpDump,
						SubjectCategory,
						NewCategory(CategoryCoiledCompression),
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
			}),
			Branches: []*Branch{
				NewBranch(
					NewConditionGroup(BooleanTypeAnd, []Condition{
						*NewCondition(
							ConditionIsTrue,
							ConditionOperand{Subject: *NewSubject(
								SourcePumpDump,
								SubjectCategory,
								NewCategory(CategoryVerticalIgnition),
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
					}),
					NewAction(
						ActionMarket,
						trading.Buy,
						"BTC/USD",
						0,
						1,
						0,
						0,
						"",
					),
				),
			},
		}

		Convey("It should enter when compression precedes ignition", func() {
			measurements := []Measurement{
				*NewMeasurement(
					SourcePumpDump,
					"BTC/USD",
					0,
					0,
					0,
					0,
					0,
					CategoryCoiledCompression,
					RegimeTypeNone,
					PositionTypeNone,
					0,
					0,
				),
				*NewMeasurement(
					SourcePumpDump,
					"BTC/USD",
					0,
					0,
					0,
					0,
					0,
					CategoryVerticalIgnition,
					RegimeTypeNone,
					PositionTypeNone,
					0,
					0,
				),
			}

			result, err := parent.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
		})

		Convey("It should not enter when ignition precedes compression", func() {
			measurements := []Measurement{
				*NewMeasurement(
					SourcePumpDump,
					"BTC/USD",
					0,
					0,
					0,
					0,
					0,
					CategoryVerticalIgnition,
					RegimeTypeNone,
					PositionTypeNone,
					0,
					0,
				),
				*NewMeasurement(
					SourcePumpDump,
					"BTC/USD",
					0,
					0,
					0,
					0,
					0,
					CategoryCoiledCompression,
					RegimeTypeNone,
					PositionTypeNone,
					0,
					0,
				),
			}

			result, err := parent.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(result, ShouldBeNil)
		})

		Convey("It should not enter when compression is the latest tick alone", func() {
			measurements := []Measurement{
				*NewMeasurement(
					SourcePumpDump,
					"BTC/USD",
					0,
					0,
					0,
					0,
					0,
					CategoryCoiledCompression,
					RegimeTypeNone,
					PositionTypeNone,
					0,
					0,
				),
			}

			result, err := parent.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(result, ShouldBeNil)
		})
	})
}

func TestConditionIsFalseDoesNotBurnTimeline(t *testing.T) {
	Convey("Given an AND group with an is_false veto", t, func() {
		group := NewConditionGroup(BooleanTypeAnd, []Condition{
			*NewCondition(
				ConditionIsTrue,
				ConditionOperand{Subject: *NewSubject(
					SourcePumpDump,
					SubjectCategory,
					NewCategory(CategoryCoiledCompression),
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
				ConditionIsFalse,
				ConditionOperand{Subject: *NewSubject(
					SourceDepthFlow,
					SubjectCategory,
					NewCategory(CategorySpoofTrap),
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
			*NewMeasurement(
				SourceDepthFlow,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategorySpoofTrap,
				RegimeTypeNone,
				PositionTypeNone,
				0,
				0,
			),
			*NewMeasurement(
				SourcePumpDump,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryCoiledCompression,
				RegimeTypeNone,
				PositionTypeNone,
				0,
				0,
			),
		}

		Convey("It should fail when spoof trap is present anywhere in the timeline", func() {
			matched, matchIndex, err := group.EvaluateIndexed(measurements, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeFalse)
			So(matchIndex, ShouldEqual, -1)
		})
	})
}
