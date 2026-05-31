package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func testMeasurement(
	category perspectives.CategoryType,
	snr float64,
) perspectives.Measurement {
	return perspectives.Measurement{Category: category, SNR: snr}
}

func trendMeasurements() []perspectives.Measurement {
	return []perspectives.Measurement{
		testMeasurement(perspectives.CategoryRiskOnSurge, 1.3),
		testMeasurement(perspectives.CategoryEndogenousAlpha, 1.4),
		testMeasurement(perspectives.CategoryFrenzy, 1.2),
		testMeasurement(perspectives.CategoryAggressiveDrive, 1.6),
	}
}

func TestDecisions(t *testing.T) {
	convey.Convey("Given measurements that satisfy multiple playbooks", t, func() {
		measurements := append(
			trendMeasurements(),
			testMeasurement(perspectives.CategoryAggressiveDrive, 1.6),
		)

		convey.Convey("It should return every actionable perspective in registry order", func() {
			decisions := Decisions(measurements, nil)

			convey.So(decisions, convey.ShouldHaveLength, 2)
			convey.So(decisions[0].Name, convey.ShouldEqual, "trend")
			convey.So(decisions[1].Name, convey.ShouldEqual, "drive")
			convey.So(decisions[0].Action, convey.ShouldEqual, perspectives.ActionEnter)
		})
	})
}

func TestExitDecisions(t *testing.T) {
	convey.Convey("Given a held trend position and an active reversal", t, func() {
		measurements := []perspectives.Measurement{
			testMeasurement(perspectives.CategoryActiveReversal, 1.5),
		}
		observations := []perspectives.ObservationType{perspectives.ObservationHolding}

		convey.Convey("It should return a stop from the opening playbook", func() {
			exits := ExitDecisions(measurements, observations, "trend", true)
			action := MostUrgentExit(exits)

			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, perspectives.ActionStopLoss)
		})
	})
}

func TestEntryCategoriesForPlaybook(t *testing.T) {
	convey.Convey("Given a generated playbook registry", t, func() {
		document := perspectives.Document{Version: 1, Playbooks: []perspectives.PlaybookSpec{{
			Name:   "drive",
			Regime: "trending",
			Policy: "drive",
			Deny: []perspectives.BranchSpec{{
				Category: "toxic_bluff",
				Action:   "deny",
			}},
			Entry: []perspectives.BranchSpec{{
				Metric:    perspectives.MetricScoreCostRatio,
				Condition: ">=",
				Value:     float64Ptr(1),
				Branches: []perspectives.BranchSpec{{
					Category: "vertical_ignition",
					Action:   "enter",
				}},
			}},
			Exit: []perspectives.BranchSpec{{
				Category: "active_reversal",
				Action:   "stop_loss",
			}},
		}}}
		strategies, err := perspectives.BuildStrategies(document)
		convey.So(err, convey.ShouldBeNil)
		convey.So(SetPerspectiveRegistry(strategies), convey.ShouldBeNil)
		defer SetPerspectiveRegistry([]perspectives.Perspective{
			perspectives.NewTrendPerspective(),
			perspectives.NewDrivePerspective(),
			perspectives.NewLeadLagPerspective(),
			perspectives.NewScarcityPerspective(),
			perspectives.NewPumpPerspective(),
		})

		categories := EntryCategoriesForPlaybook("drive")

		convey.Convey("It should report the active tree categories instead of the legacy playbook map", func() {
			convey.So(categories, convey.ShouldResemble, []perspectives.CategoryType{
				perspectives.CategoryVerticalIgnition,
			})
		})
	})
}

func BenchmarkDecisions(b *testing.B) {
	measurements := trendMeasurements()

	for b.Loop() {
		_ = Decisions(measurements, nil)
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}
