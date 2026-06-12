package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestTreeEvaluateContinuing(t *testing.T) {
	Convey("Given a parent branch that needs a later timeline tick", t, func() {
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

		compressionOnly := []Measurement{
			NewMeasurement(
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

		Convey("It should park when the child timeline is not available yet", func() {
			evaluation, walkState, err := tree.EvaluateContinuing(compressionOnly, nil, nil, nil, nil)

			So(err, ShouldBeNil)
			So(evaluation, ShouldBeNil)
			So(walkState, ShouldNotBeNil)
			So(walkState.BranchPath, ShouldResemble, []int{0})
			So(walkState.MatchIndex, ShouldEqual, 0)
		})

		Convey("It should continue from the parked branch on the next spectrum", func() {
			_, walkState, err := tree.EvaluateContinuing(compressionOnly, nil, nil, nil, nil)

			So(err, ShouldBeNil)
			So(walkState, ShouldNotBeNil)

			compressionThenIgnition := []Measurement{
				NewMeasurement(
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
				NewMeasurement(
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

			evaluation, nextState, err := tree.EvaluateContinuing(
				compressionThenIgnition,
				nil,
				walkState,
				nil,
				nil,
			)

			So(err, ShouldBeNil)
			So(nextState, ShouldBeNil)
			So(evaluation, ShouldNotBeNil)
			So(evaluation.Action.Side, ShouldEqual, trading.Buy)
		})
	})
}

func TestTreeEvaluateContinuingExpiresParkedState(t *testing.T) {
	Convey("Given an expired parked branch", t, func() {
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

		tree := &Tree{
			Branches:        []*Branch{parent},
			entryTransitTTL: time.Nanosecond,
		}
		compressionOnly := []Measurement{
			NewMeasurement(
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
		ignitionOnly := []Measurement{
			NewMeasurement(
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

		_, walkState, err := tree.EvaluateContinuing(compressionOnly, nil, nil, nil, nil)

		So(err, ShouldBeNil)
		So(walkState, ShouldNotBeNil)

		walkState.ParkedAt = time.Now().Add(-time.Second)
		evaluation, nextState, err := tree.EvaluateContinuing(
			ignitionOnly,
			nil,
			walkState,
			nil,
			nil,
		)

		Convey("It should reject the parked branch instead of continuing it", func() {
			So(err, ShouldBeNil)
			So(evaluation, ShouldBeNil)
			So(nextState, ShouldBeNil)
		})
	})
}

func TestSourceIndex(t *testing.T) {
	Convey("Given the fixed measurement spectrum", t, func() {
		Convey("It should map every spectrum source to a unique slot", func() {
			seen := make(map[int]SourceType, SourceCount)

			for _, source := range SpectrumSources {
				index, err := SourceIndex(source)

				So(err, ShouldBeNil)
				So(seen[index], ShouldBeEmpty)
				seen[index] = source
			}

			So(len(seen), ShouldEqual, SourceCount)
		})

		Convey("It should reject unknown sources", func() {
			_, err := SourceIndex(SourceNone)

			So(err, ShouldNotBeNil)
		})
	})
}
