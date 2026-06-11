package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestConditionGroupExplainFailure(t *testing.T) {
	Convey("Given a category mismatch", t, func() {
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
		})

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
		}

		reason := group.ExplainFailure(measurements, nil)

		Convey("It should name the expected and actual categories", func() {
			So(reason, ShouldContainSubstring, "coiled_compression")
			So(reason, ShouldContainSubstring, "vertical_ignition")
		})
	})
}

func TestTreeEvaluateContinuingTrace(t *testing.T) {
	Convey("Given a parent branch that parks waiting for a child tick", t, func() {
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

		tree := &Tree{Branches: []*Branch{parent}}
		trace := &WalkTrace{Symbol: "BTC/USD"}

		compressionOnly := []Measurement{
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

		_, walkState, err := tree.EvaluateContinuing(compressionOnly, nil, nil, trace)

		Convey("It should record match and park on the parent branch", func() {
			So(err, ShouldBeNil)
			So(walkState, ShouldNotBeNil)
			So(len(trace.Steps), ShouldEqual, 2)
			So(trace.Steps[0].Outcome, ShouldEqual, StepMatched)
			So(trace.Steps[0].Path, ShouldResemble, []int{0})
			So(trace.Steps[1].Outcome, ShouldEqual, StepParked)
			So(trace.ActivePath, ShouldResemble, []int{0})
		})
	})
}
