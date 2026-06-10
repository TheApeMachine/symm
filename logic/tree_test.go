package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestNewTree(t *testing.T) {
	Convey("Given the embedded default tree", t, func() {
		tree, err := NewTree()

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
					firstAction,
				),
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

			evaluation := tree.Evaluate(measurements, NewHoldings())

			So(evaluation, ShouldNotBeNil)
			So(evaluation.Key, ShouldEqual, "0")
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

			So(tree.Evaluate(measurements, NewHoldings()), ShouldBeNil)
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

func TestTreeEvaluateTracedIgnitionBottleneck(t *testing.T) {
	Convey("Given measurements that pass ignition pump but fail hawkes", t, func() {
		viper.Set("trading.entry.confidence_baseline", 0.55)
		viper.Set("trading.entry.turbulence_confidence_scale", 0.30)
		viper.Set("trading.entry.surprise_baseline", 1.0)
		viper.Set("trading.entry.turbulence_surprise_scale", 0.25)

		tree, err := NewTree()

		So(err, ShouldBeNil)

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
				0.72,
				1.4,
			),
			*NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryOrganic,
				RegimeTypeNone,
				PositionTypeNone,
				0.35,
				0.39,
			),
			*NewMeasurement(
				SourceLiquidity,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryMedianDepth,
				RegimeTypeNone,
				PositionTypeNone,
				0.33,
				1.97,
			),
		}

		trace := &EvalTrace{}

		Convey("It should keep the ignition-path bottleneck for audit", func() {
			So(tree.Evaluate(measurements, NewHoldings()), ShouldBeNil)

			bottleneck := trace.Bottleneck()

			So(bottleneck, ShouldNotBeNil)
			So(bottleneck.Key, ShouldEqual, "5/0/0")
			So(trace.FailedConditionLabels(), ShouldContain, "hawkes.frenzy")
		})
	})
}
