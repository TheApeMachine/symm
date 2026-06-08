package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestBranchEvaluate(t *testing.T) {
	Convey("Given a branch with a Hawkes frenzy rule", t, func() {
		group := NewConditionGroup(BooleanTypeAnd, []Condition{
			*NewCondition(
				ConditionIsTrue,
				ConditionOperand{Subject: *NewSubject(
					SourceHawkes,
					SubjectCategory,
					NewCategory(CategoryFrenzy, 0, 0),
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

		action := NewAction(
			ActionMarket,
			trading.Buy,
			"BTC/USD",
			0,
			1,
			0,
			0,
			"",
		)

		branch := NewBranch(group, action)

		Convey("It should return nil when conditions miss", func() {
			measurements := []Measurement{
				*NewMeasurement(
					SourceHawkes,
					"BTC/USD",
					0,
					0,
					0,
					0,
					0,
					CategorySaturation,
					RegimeTypeNone,
					PositionTypeNone,
					0,
					0,
				),
			}

			result, err := branch.Evaluate(measurements)

			So(err, ShouldBeNil)
			So(result, ShouldBeNil)
		})

		Convey("It should return the action when conditions hit", func() {
			measurements := []Measurement{
				*NewMeasurement(
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

			result, err := branch.Evaluate(measurements)

			So(err, ShouldBeNil)
			So(result, ShouldEqual, action)
		})
	})

	Convey("Given a branch with nested branches", t, func() {
		parentAction := NewAction(
			ActionMarket,
			trading.Buy,
			"BTC/USD",
			0,
			1,
			0,
			0,
			"",
		)

		childAction := NewAction(
			ActionLimit,
			trading.Sell,
			"BTC/USD",
			100,
			1,
			0,
			0,
			"",
		)

		parent := &Branch{
			ConditionGroup: NewConditionGroup(BooleanTypeAnd, []Condition{
				*NewCondition(
					ConditionIsTrue,
					ConditionOperand{Subject: *NewSubject(
						SourceHawkes,
						SubjectCategory,
						NewCategory(CategoryFrenzy, 0, 0),
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
			Action: parentAction,
			Branches: []*Branch{
				NewBranch(
					NewConditionGroup(BooleanTypeAnd, []Condition{
						*NewCondition(
							ConditionIsTrue,
							ConditionOperand{Subject: *NewSubject(
								SourceToxicity,
								SubjectCategory,
								NewCategory(CategoryToxicBluff, 0, 0),
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
					childAction,
				),
			},
		}

		Convey("It should return the first matching child action", func() {
			measurements := []Measurement{
				*NewMeasurement(
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
				*NewMeasurement(
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
					0,
					0,
				),
			}

			result, err := parent.Evaluate(measurements)

			So(err, ShouldBeNil)
			So(result, ShouldEqual, childAction)
		})

		Convey("It should fall back to the parent action when no child matches", func() {
			measurements := []Measurement{
				*NewMeasurement(
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

			result, err := parent.Evaluate(measurements)

			So(err, ShouldBeNil)
			So(result, ShouldEqual, parentAction)
		})
	})
}
