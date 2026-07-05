package market

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestStoryUpdate(t *testing.T) {
	Convey("Given a Story with a candidate branch", t, func() {
		story := &Story{
			symbols: &sync.Map{},
			dirty:   &sync.Map{},
			tree: &logic.Tree{
				Branches: []*logic.Branch{{
					ConditionGroup: &logic.ConditionGroup{
						Boolean: logic.BooleanTypeAnd,
						Conditions: []logic.Condition{{
							Type: logic.ConditionIsTrue,
							Left: logic.ConditionOperand{
								Type:     logic.SubjectCategory,
								Source:   logic.SourcePumpDump,
								Category: logic.NewCategory(logic.CategoryVerticalIgnition),
							},
						}},
					},
					Action: &logic.Action{
						Type: logic.ActionMarket,
						Side: logic.SideBuy,
					},
				}},
			},
		}
		measurement := &logic.Measurement{
			Source: logic.SourcePumpDump,
			Symbol: "BTC/USD",
			At:     time.Now(),
			Distribution: map[logic.CategoryType]float64{
				logic.CategoryVerticalIgnition: 0.8,
			},
			Confidence:    0.8,
			EntryBaseline: 0.25,
			ExitBaseline:  0.25,
		}

		Convey("When matching measurements are updated and actions are requested", func() {
			err := story.Update([]*logic.Measurement{measurement})
			actions, actionsErr := story.Actions(nil)

			Convey("Then it should emit a scoped candidate action", func() {
				So(err, ShouldBeNil)
				So(actionsErr, ShouldBeNil)
				So(actions, ShouldHaveLength, 1)
				So(actions[0].Symbol, ShouldEqual, "BTC/USD")
				So(actions[0].Side, ShouldEqual, logic.SideBuy)
				So(actions[0].Story.Status, ShouldEqual, "candidate")
				So(actions[0].Story.Symbol, ShouldEqual, "BTC/USD")
				So(actions[0].Story.Source, ShouldEqual, logic.SourcePumpDump)
				So(actions[0].Story.Category, ShouldEqual, logic.CategoryVerticalIgnition)
				So(actions[0].EntryScore, ShouldAlmostEqual, (0.8-0.25)/(1-0.25), 1e-12)
				So(actions[0].EntryConfidence, ShouldEqual, 0.8)
			})

			Convey("Then it should stamp story evaluation on the source measurement", func() {
				So(measurement.Story.Evaluated, ShouldBeTrue)
				So(measurement.Story.Candidates, ShouldEqual, 1)
			})
		})
	})
}

func TestStoryActionsWithTrace(t *testing.T) {
	Convey("Given a Story with a terminal branch and no action", t, func() {
		story := &Story{
			symbols: &sync.Map{},
			dirty:   &sync.Map{},
			tree: &logic.Tree{
				Branches: []*logic.Branch{{
					ID:       "setup.clean_ignition",
					Terminal: "early_daily_expansion_observed",
					ConditionGroup: &logic.ConditionGroup{
						Boolean: logic.BooleanTypeAnd,
						Conditions: []logic.Condition{{
							Type: logic.ConditionIsTrue,
							Left: logic.ConditionOperand{
								Type:     logic.SubjectCategory,
								Source:   logic.SourcePumpDump,
								Category: logic.NewCategory(logic.CategoryVerticalIgnition),
							},
						}},
					},
				}},
			},
		}
		measurement := &logic.Measurement{
			Source: logic.SourcePumpDump,
			Symbol: "BTC/USD",
			At:     time.Now(),
			Distribution: map[logic.CategoryType]float64{
				logic.CategoryVerticalIgnition: 0.8,
			},
			Confidence:    0.8,
			EntryBaseline: 0.25,
			ExitBaseline:  0.25,
		}

		Convey("When matching measurements are evaluated with trace output", func() {
			err := story.Update([]*logic.Measurement{measurement})
			actions, traces, traceErr := story.ActionsWithTrace(nil)

			Convey("Then it should emit no candidate action", func() {
				So(err, ShouldBeNil)
				So(traceErr, ShouldBeNil)
				So(actions, ShouldBeEmpty)
			})

			Convey("Then it should return the terminal measurement trace", func() {
				So(traces, ShouldHaveLength, 1)
				So(traces[0], ShouldEqual, measurement)
				So(measurement.Story.Terminal, ShouldEqual, "early_daily_expansion_observed")
				So(measurement.Story.TerminalBranchID, ShouldEqual, "setup.clean_ignition")
			})
		})
	})
}

func TestStoryUpdateRejectsIncompleteMeasurements(t *testing.T) {
	Convey("Given a Story and an incomplete measurement", t, func() {
		story := &Story{
			symbols: &sync.Map{},
			dirty:   &sync.Map{},
			tree: &logic.Tree{
				Branches: []*logic.Branch{{
					ConditionGroup: &logic.ConditionGroup{
						Boolean: logic.BooleanTypeAnd,
						Conditions: []logic.Condition{{
							Type: logic.ConditionIsTrue,
							Left: logic.ConditionOperand{
								Type:     logic.SubjectCategory,
								Source:   logic.SourcePumpDump,
								Category: logic.NewCategory(logic.CategoryVerticalIgnition),
							},
						}},
					},
					Action: &logic.Action{
						Type: logic.ActionMarket,
						Side: logic.SideBuy,
					},
				}},
			},
		}
		measurement := &logic.Measurement{
			Source: logic.SourcePumpDump,
			Symbol: "BTC/USD",
			At:     time.Now(),
			Distribution: map[logic.CategoryType]float64{
				logic.CategoryVerticalIgnition: 0.8,
			},
		}

		Convey("When the measurement is updated and actions are requested", func() {
			err := story.Update([]*logic.Measurement{measurement})
			actions, traces, traceErr := story.ActionsWithTrace(nil)

			Convey("Then it should not enter the playbook ring", func() {
				So(err, ShouldNotBeNil)
				So(traceErr, ShouldBeNil)
				So(actions, ShouldBeEmpty)
				So(traces, ShouldBeEmpty)
				So(measurement.Story.Evaluated, ShouldBeFalse)
			})
		})
	})
}
