package strategy

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPlanner(t *testing.T) {
	Convey("Given a Planner", t, func() {
		planner := NewPlanner(nil)

		Convey("When evaluating an empty thesis", func() {
			thesis := NewThesis()
			intents := planner.Update(thesis)

			Convey("Then no intents are produced", func() {
				So(len(intents), ShouldEqual, 0)
			})
		})

		Convey("When evaluating a thesis with strong buy forecasts", func() {
			thesis := NewThesis()
			forecasts := types.Forecasts{
				ExecutableReturn: 0.05,
				Uncertainty:      0.2,
			}
			thesis.AddEvidence("BTCUSD", "manifold_forecasts", forecasts)

			intents := planner.Update(thesis)

			Convey("Then a buy intent is produced and a decision is appended", func() {
				So(len(intents), ShouldEqual, 1)
				So(intents[0].Action, ShouldEqual, ActionBuy)
				So(intents[0].Symbol, ShouldEqual, "BTCUSD")
				So(intents[0].Edge.Cmp(decimal.NewFromFloat64(0.05)), ShouldEqual, 0)

				decision, ok := thesis.Evidence("BTCUSD", "decision")
				So(ok, ShouldBeTrue)
				So(decision.(Decision).Action, ShouldEqual, ActionBuy)
			})
		})

		Convey("When evaluating a thesis with high uncertainty", func() {
			thesis := NewThesis()
			forecasts := types.Forecasts{
				ExecutableReturn: 0.05,
				Uncertainty:      0.9,
			}
			thesis.AddEvidence("BTCUSD", "manifold_forecasts", forecasts)

			intents := planner.Update(thesis)

			Convey("Then a hold intent is produced", func() {
				So(len(intents), ShouldEqual, 1)
				So(intents[0].Action, ShouldEqual, ActionHold)

				decision, ok := thesis.Evidence("BTCUSD", "decision")
				So(ok, ShouldBeTrue)
				So(decision.(Decision).Action, ShouldEqual, ActionHold)
			})
		})
	})
}

func BenchmarkPlannerUpdate(b *testing.B) {
	planner := NewPlanner(nil)
	thesis := NewThesis()
	forecasts := types.Forecasts{
		ExecutableReturn: 0.05,
		Uncertainty:      0.2,
	}
	thesis.AddEvidence("BTCUSD", "manifold_forecasts", forecasts)

	b.ResetTimer()
	for b.Loop() {
		_ = planner.Update(thesis)
	}
}
