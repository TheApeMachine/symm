package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestNewTree(t *testing.T) {
	Convey("Given the embedded default tree", t, func() {
		tree, err := NewTree(nil)

		Convey("It should load without error", func() {
			So(err, ShouldBeNil)
			So(tree, ShouldNotBeNil)
			So(len(tree.Branches), ShouldBeGreaterThan, 0)
		})
	})
}

func TestTreeEvaluate(t *testing.T) {
	Convey("Given a tree with ordered branches", t, func() {
		firstAction := NewAction(
			ActionMarket,
			trading.Buy,
			"BTC/USD",
			0,
			1,
			0,
			0,
			"",
		)

		secondAction := NewAction(
			ActionLimit,
			trading.Sell,
			"BTC/USD",
			100,
			1,
			0,
			0,
			"",
		)

		tree := &Tree{
			Branches: []*Branch{
				NewBranch(
					NewConditionGroup(BooleanTypeAnd, []Condition{
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
					}),
					firstAction,
				),
				NewBranch(
					NewConditionGroup(BooleanTypeAnd, []Condition{
						*NewCondition(
							ConditionIsTrue,
							ConditionOperand{Subject: *NewSubject(
								SourceToxicity,
								SubjectCategory,
								NewCategory(CategoryToxicBluff),
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
					secondAction,
				),
			},
		}

		Convey("It should return the first matching branch action", func() {
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

			evaluation, err := tree.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(evaluation, ShouldNotBeNil)
			So(evaluation.Action.Type, ShouldEqual, firstAction.Type)
			So(evaluation.Action.Side, ShouldEqual, firstAction.Side)
			So(evaluation.Action.Symbol, ShouldEqual, firstAction.Symbol)
			So(evaluation.Action.Quantity, ShouldEqual, firstAction.Quantity)
		})

		Convey("It should return nil when no branch matches", func() {
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

			evaluation, err := tree.Evaluate(measurements, nil)

			So(err, ShouldBeNil)
			So(evaluation, ShouldBeNil)
		})

		Convey("It should gate entry on empty holdings", func() {
			holdingsGate := &Tree{
				Branches: []*Branch{
					NewBranch(
						NewConditionGroup(BooleanTypeAnd, []Condition{
							*NewCondition(
								ConditionIsTrue,
								ConditionOperand{Subject: *NewSubject(
									SourceNone,
									SubjectHolding,
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
			holdingsGate.Branches[0].ConditionGroup.Conditions[0].Left.Subject.Holding = &HoldingSubject{Held: false}

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

			emptyHoldings := NewHoldings()

			evaluation, err := holdingsGate.Evaluate(measurements, emptyHoldings)

			So(err, ShouldBeNil)
			So(evaluation, ShouldNotBeNil)

			filledHoldings := NewHoldings()
			filledHoldings.SetQuantity("BTC/USD", 1)

			evaluation, err = holdingsGate.Evaluate(measurements, filledHoldings)

			So(err, ShouldBeNil)
			So(evaluation, ShouldBeNil)
		})
	})
}

func BenchmarkTreeEvaluate(b *testing.B) {
	tree := &Tree{
		Branches: []*Branch{
			NewBranch(
				NewConditionGroup(BooleanTypeAnd, []Condition{
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
			0.8,
			2.5,
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
			0.7,
			1.5,
		),
	}

	b.ResetTimer()

	for b.Loop() {
		tree.Evaluate(measurements, NewHoldings())
	}
}
