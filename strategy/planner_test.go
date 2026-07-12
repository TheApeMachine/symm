package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPlanner(t *testing.T) {
	Convey("Given a Planner", t, func() {
		planner := NewPlanner(nil, nil)

		Convey("When evaluating an empty thesis", func() {
			thesis := NewThesis()
			intents := planner.Update(thesis)

			Convey("Then no intents are produced", func() {
				So(len(intents), ShouldEqual, 0)
			})
		})

		Convey("When evaluating a thesis with strong buy forecasts", func() {
			thesis := NewThesis()
			forecasts := testForecast(0.05, 0.0)
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

		Convey("When execution friction consumes the expected return", func() {
			thesis := NewThesis()
			forecasts := testForecast(0.05, 0.06)
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
	planner := NewPlanner(nil, nil)
	thesis := NewThesis()
	forecasts := testForecast(0.05, 0.0)
	thesis.AddEvidence("BTCUSD", "manifold_forecasts", forecasts)

	b.ResetTimer()
	for b.Loop() {
		_ = planner.Update(thesis)
	}
}

func testForecast(expectedReturn float64, expectedSpread float64) types.Forecasts {
	return types.Forecasts{
		Source:         "manifold_forecast",
		Symbol:         "BTCUSD",
		At:             time.Unix(1, 0),
		SourceEpoch:    1,
		HorizonEvents:  1,
		ExpiresEpoch:   2,
		Target:         "next_l3_epoch_mid_log_return",
		ModelVersion:   "test",
		Ready:          true,
		ExpectedReturn: expectedReturn,
		ExpectedSpread: expectedSpread,
		Confidence:     0.8,
	}
}
