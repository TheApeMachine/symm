package market

import (
	"sync"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStoryUpdate(t *testing.T) {
	Convey("Given a Story with a candidate branch", t, func() {
		story := &Story{
			symbols: &sync.Map{},
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
		measurement := datura.Acquire("measurement", datura.APPJSON)
		measurement.WithRole("measurement")
		measurement.WithScope("BTC/USD")
		So(measurement.SetOrigin(string(logic.SourcePumpDump)), ShouldBeNil)
		measurement.MergeOutput("value", float64(logic.CategoryIndex(logic.CategoryVerticalIgnition)))
		measurement.MergeOutput("confidence", 0.8)

		Convey("When matching measurements are updated and actions are requested", func() {
			story.Update([]*datura.Artifact{measurement})
			actions := story.Actions(nil)

			Convey("Then it should emit a scoped candidate artifact", func() {
				So(actions, ShouldHaveLength, 1)

				symbol, err := actions[0].Scope()
				So(err, ShouldBeNil)
				So(symbol, ShouldEqual, "BTC/USD")

				side, err := actions[0].Role()
				So(err, ShouldBeNil)
				So(side, ShouldEqual, "buy")

				So(datura.Peek[string](actions[0], "symbol"), ShouldEqual, "BTC/USD")
				So(datura.Peek[string](actions[0], "journey", "story", "status"), ShouldEqual, "candidate")
			})

			Convey("Then it should stamp story evaluation on the source measurement", func() {
				So(datura.Peek[bool](measurement, "journey", "story", "evaluated"), ShouldBeTrue)
				So(datura.Peek[float64](measurement, "journey", "story", "candidates"), ShouldEqual, 1)
			})
		})
	})
}

func TestStoryActionsWithTrace(t *testing.T) {
	Convey("Given a Story with a terminal branch and no action", t, func() {
		story := &Story{
			symbols: &sync.Map{},
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
		measurement := datura.Acquire("measurement", datura.APPJSON)
		measurement.WithRole("measurement")
		measurement.WithScope("BTC/USD")
		So(measurement.SetOrigin(string(logic.SourcePumpDump)), ShouldBeNil)
		measurement.MergeOutput("value", float64(logic.CategoryIndex(logic.CategoryVerticalIgnition)))
		measurement.MergeOutput("confidence", 0.8)

		Convey("When matching measurements are evaluated with trace output", func() {
			story.Update([]*datura.Artifact{measurement})
			actions, traces := story.ActionsWithTrace(nil)

			Convey("Then it should emit no candidate action", func() {
				So(actions, ShouldBeEmpty)
			})

			Convey("Then it should return the terminal measurement trace", func() {
				So(traces, ShouldHaveLength, 1)
				So(traces[0], ShouldEqual, measurement)
				So(datura.Peek[string](measurement, "journey", "story", "terminal"), ShouldEqual, "early_daily_expansion_observed")
				So(datura.Peek[string](measurement, "journey", "story", "terminal_branch_id"), ShouldEqual, "setup.clean_ignition")
			})
		})
	})
}
