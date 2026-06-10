package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestTreeStatsDecisionTreeFrame(t *testing.T) {
	Convey("Given an instrumented branch evaluation", t, func() {
		branch := NewBranch(
			NewConditionGroup(BooleanTypeAnd, []Condition{
				*NewCondition(
					ConditionIsTrue,
					ConditionOperand{Subject: Subject{
						Source:   SourceHawkes,
						Type:     SubjectCategory,
						Category: NewCategory(CategoryFrenzy, 0, 0),
					}},
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
		)

		stats := NewTreeStats([]*Branch{branch}, 4)

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

		stats.BeginEvaluation()
		stats.Reach("0")
		stats.RecordConditions(
			"0",
			branch.ConditionGroup,
			measurements,
			NewEvalContext(measurements, nil),
		)
		stats.Hold("0")
		stats.RecordAction("BTC/USD", &Evaluation{
			Action: branch.Action,
			Key:    "0",
		}, "submitted", "")

		frame := stats.DecisionTreeFrame()

		Convey("It should publish a decision_tree frame", func() {
			So(frame["chart"], ShouldEqual, "decision_tree")
			So(frame["evaluations"], ShouldEqual, 1)

			nodes, ok := frame["nodes"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(nodes), ShouldEqual, 1)
			So(nodes[0]["reached"], ShouldEqual, 1)
			So(nodes[0]["held"], ShouldEqual, 1)

			recent, ok := frame["recent"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(recent), ShouldEqual, 1)
		})

		Convey("It should keep accumulating across publishes", func() {
			stats.BeginEvaluation()
			stats.Reach("0")
			stats.Hold("0")

			second := stats.DecisionTreeFrame()

			So(second["evaluations"], ShouldEqual, 2)

			nodes, ok := second["nodes"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(nodes[0]["reached"], ShouldEqual, 2)
			So(nodes[0]["held"], ShouldEqual, 2)
		})
	})
}

func TestTreeStatsObserveStoryTick(t *testing.T) {
	Convey("Given story tick observations", t, func() {
		stats := NewTreeStats(nil, 0)

		stats.ObserveStoryTick()
		stats.ObserveStoryTick()

		frame := stats.DecisionTreeFrame()

		Convey("It should expose cumulative story ticks on the frame", func() {
			So(frame["story_ticks"], ShouldEqual, 2)
			So(frame["evaluations"], ShouldEqual, 0)
		})
	})
}

func TestTreeStatsDecisionTreeFrameEmptyRecent(t *testing.T) {
	Convey("Given stats with no recorded actions", t, func() {
		branch := NewBranch(
			NewConditionGroup(BooleanTypeAnd, []Condition{
				*NewCondition(
					ConditionIsTrue,
					ConditionOperand{Subject: Subject{
						Source:   SourceHawkes,
						Type:     SubjectCategory,
						Category: NewCategory(CategoryFrenzy, 0, 0),
					}},
					ConditionOperand{},
				),
			}),
			nil,
		)

		stats := NewTreeStats([]*Branch{branch}, 4)
		frame := stats.DecisionTreeFrame()

		Convey("It should emit an empty recent slice, not nil", func() {
			recent, ok := frame["recent"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(recent, ShouldNotBeNil)
			So(len(recent), ShouldEqual, 0)
		})
	})
}
