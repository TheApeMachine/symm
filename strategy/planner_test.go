package strategy

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func TestPlannerStatus(t *testing.T) {
	Convey("Given a newly constructed Planner", t, func() {
		planner := NewPlanner(readyPlannerGate())

		Convey("When its construction status is read", func() {
			Convey("Then it is ready because construction has no deferred work", func() {
				So(planner.Status(), ShouldEqual, types.READY)
			})
		})
	})
}

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a ready Planner", t, func() {
		planner := NewPlanner(readyPlannerGate())

		Convey("When the thesis has no forecast evidence", func() {
			thesis := NewThesis()
			thesis.AddEvidence("BTC/USD", "ticker", 1.0)

			intents, err := planner.Update(thesis)

			Convey("Then no intent or artificial hold decision is produced", func() {
				So(err, ShouldBeNil)
				So(intents, ShouldBeEmpty)
				So(thesis.Decisions("BTC/USD"), ShouldBeEmpty)
			})
		})

		Convey("When the current forecast is invalid", func() {
			thesis := NewThesis()
			forecast := testForecast("BTC/USD", 1, 0.05, 0.0)
			forecast.Ready = false
			thesis.AddEvidence("BTC/USD", "manifold_forecasts", forecast)

			intents, err := planner.Update(thesis)

			Convey("Then absence of evaluable evidence is not recorded as a hold", func() {
				So(err, ShouldBeNil)
				So(intents, ShouldBeEmpty)
				So(thesis.Decisions("BTC/USD"), ShouldBeEmpty)
			})
		})

		Convey("When finite forecast fields overflow aggregate utility", func() {
			thesis := NewThesis()
			forecast := testForecast("BTC/USD", 1, -math.MaxFloat64, 0.0)
			forecast.ExpectedFees = math.MaxFloat64
			thesis.AddEvidence("BTC/USD", "manifold_forecasts", forecast)

			intents, err := planner.Update(thesis)

			Convey("Then non-finite utility fails explicitly before an intent", func() {
				So(err, ShouldNotBeNil)
				So(intents, ShouldBeEmpty)
				So(thesis.Decisions("BTC/USD"), ShouldBeEmpty)
			})
		})

		Convey("When a positive executable return is evaluated twice", func() {
			thesis := NewThesis()
			forecast := testForecast("BTC/USD", 1, 0.05, 0.0)
			thesis.AddEvidence("BTC/USD", "manifold_forecasts", forecast)

			first, firstErr := planner.Update(thesis)
			second, secondErr := planner.Update(thesis)

			Convey("Then one buy intent and one provenance-complete decision exist", func() {
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(first, ShouldHaveLength, 1)
				So(first[0].Action, ShouldEqual, ActionBuy)
				So(first[0].Symbol, ShouldEqual, "BTC/USD")
				So(first[0].Edge.Cmp(decimal.NewFromFloat64(0.05)), ShouldEqual, 0)
				So(second, ShouldBeEmpty)

				decisions := thesis.Decisions("BTC/USD")
				So(decisions, ShouldHaveLength, 1)
				So(decisions[0].Forecast, ShouldResemble, forecast)
				So(decisions[0].Alternatives, ShouldResemble, []Alternative{
					{Action: ActionBuy, Utility: 0.05},
					{Action: ActionHold, Utility: 0.0},
				})
			})
		})

		Convey("When a newer forecast epoch arrives", func() {
			thesis := NewThesis()
			thesis.AddEvidence(
				"BTC/USD",
				"manifold_forecasts",
				testForecast("BTC/USD", 1, 0.02, 0.0),
			)
			first, firstErr := planner.Update(thesis)
			thesis.AddEvidence(
				"BTC/USD",
				"manifold_forecasts",
				testForecast("BTC/USD", 2, 0.03, 0.0),
			)

			second, secondErr := planner.Update(thesis)

			Convey("Then each distinct epoch is evaluated exactly once", func() {
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(first, ShouldHaveLength, 1)
				So(second, ShouldHaveLength, 1)
				So(thesis.Decisions("BTC/USD"), ShouldHaveLength, 2)
			})
		})

		Convey("When reported friction consumes the expected return", func() {
			thesis := NewThesis()
			forecast := testForecast("BTC/USD", 1, 0.05, 0.06)
			thesis.AddEvidence("BTC/USD", "manifold_forecasts", forecast)

			intents, err := planner.Update(thesis)

			Convey("Then hold is journaled but no no-op broker intent is emitted", func() {
				So(err, ShouldBeNil)
				So(intents, ShouldBeEmpty)

				decisions := thesis.Decisions("BTC/USD")
				So(decisions, ShouldHaveLength, 1)
				So(decisions[0].Action, ShouldEqual, ActionHold)
				So(decisions[0].Utility, ShouldEqual, 0.0)
			})
		})

		Convey("When several symbols have positive executable return", func() {
			thesis := NewThesis()
			forecasts := []types.Forecasts{
				testForecast("Z/USD", 1, 0.08, 0.0),
				testForecast("B/USD", 1, 0.03, 0.0),
				testForecast("A/USD", 1, 0.03, 0.0),
			}

			for _, forecast := range forecasts {
				thesis.AddEvidence(
					forecast.Symbol,
					"manifold_forecasts",
					forecast,
				)
			}

			intents, err := planner.Update(thesis)

			Convey("Then utility ranks candidates and symbol breaks exact ties", func() {
				So(err, ShouldBeNil)
				So(intents, ShouldHaveLength, 3)
				So(intents[0].Symbol, ShouldEqual, "Z/USD")
				So(intents[1].Symbol, ShouldEqual, "A/USD")
				So(intents[2].Symbol, ShouldEqual, "B/USD")
			})
		})
	})
}

func BenchmarkPlannerUpdate(b *testing.B) {
	gate := readyPlannerGate()
	const symbols = 1455
	planner := NewPlanner(gate)
	thesis := NewThesis()

	for index := range symbols {
		symbol := fmt.Sprintf("ASSET-%04d/USD", index)
		forecast := testForecast(
			symbol,
			1,
			float64((index%100)+1)/10_000,
			0.0,
		)
		thesis.AddEvidence(symbol, "manifold_forecasts", forecast)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()
		thesis.decisions = NewDecisionJournal()
		b.StartTimer()
		intents, err := planner.Update(thesis)

		if err != nil {
			b.Fatal(err)
		}

		if len(intents) != symbols {
			b.Fatalf("expected %d intents, got %d", symbols, len(intents))
		}
	}
}

type plannerGate bool

func (gate plannerGate) Ready(system.StageType) bool {
	return bool(gate)
}

func readyPlannerGate() stageGate {
	return plannerGate(true)
}

func testForecast(
	symbol string,
	sourceEpoch uint64,
	expectedReturn float64,
	expectedSpread float64,
) types.Forecasts {
	return types.Forecasts{
		Source:         "manifold_forecast",
		Symbol:         symbol,
		At:             time.Unix(int64(sourceEpoch), 0),
		SourceEpoch:    sourceEpoch,
		HorizonEvents:  1,
		ExpiresEpoch:   sourceEpoch + 1,
		Target:         "next_l3_epoch_mid_log_return",
		ModelVersion:   "test",
		Ready:          true,
		ExpectedReturn: expectedReturn,
		ExpectedSpread: expectedSpread,
		Confidence:     0.8,
	}
}
