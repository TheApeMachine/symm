package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
crossSectionProbe records the CrossSection carried by each Thesis tick.
*/
type crossSectionProbe struct {
	rows     []kraken.TickerData
	sections []*types.CrossSection
}

func (probe *crossSectionProbe) Measure(thesis *types.Thesis) *types.Thesis {
	row := probe.rows[0]
	probe.rows = probe.rows[1:]

	thesis.CrossSection.Measure([]kraken.TickerData{row})
	probe.sections = append(probe.sections, thesis.CrossSection)

	return thesis
}

func TestPlannerUpdateCreatesCrossSectionPerTick(t *testing.T) {
	Convey("Given a planner ticking twice", t, func() {
		start := time.Unix(1_700_000_000, 0)
		probe := &crossSectionProbe{
			rows: []kraken.TickerData{
				{Symbol: "BTC/USD", Last: decimal.NewFromFloat64(100), Timestamp: start},
				{Symbol: "BTC/USD", Last: decimal.NewFromFloat64(101), Timestamp: start.Add(time.Second)},
			},
		}

		planner := NewPlanner(context.Background(), nil, []types.Signal{probe}, nil)

		planner.Update()
		planner.Update()

		Convey("Then each Thesis owns its current tick's cross-section", func() {
			So(probe.sections, ShouldHaveLength, 2)
			So(probe.sections[0], ShouldNotEqual, probe.sections[1])
			So(probe.sections[0].Metrics, ShouldHaveLength, 1)
			So(probe.sections[1].Metrics, ShouldHaveLength, 1)
		})
	})
}

func TestPlannerUpdateDedupesRepeatedTimestampsAcrossSignals(t *testing.T) {
	Convey("Given two signals observing the same ticker row in one tick", t, func() {
		at := time.Unix(1_700_000_000, 0)
		row := kraken.TickerData{Symbol: "BTC/USD", Last: decimal.NewFromFloat64(100), Timestamp: at}

		observer := func(thesis *types.Thesis) *types.Thesis {
			thesis.CrossSection.Measure([]kraken.TickerData{row})
			return thesis
		}

		signals := []types.Signal{
			signalFunc(observer),
			signalFunc(observer),
		}

		planner := NewPlanner(context.Background(), nil, signals, nil)

		thesis := planner.Update()

		Convey("Then the tick contains one metric for the symbol", func() {
			So(thesis.CrossSection.Metrics, ShouldHaveLength, 1)
		})
	})
}

type signalFunc func(*types.Thesis) *types.Thesis

func (fn signalFunc) Measure(thesis *types.Thesis) *types.Thesis {
	return fn(thesis)
}

func TestPlannerDecide(t *testing.T) {
	Convey("Given calibrated forecasts for an open and unexposed symbol", t, func() {
		planner := NewPlanner(context.Background(), nil, nil, nil)
		thesis := types.NewThesis(nil)
		positionThesis := types.NewThesis(nil)
		So(positionThesis.Transition(
			"BTC/USD", types.LifecycleManaging, time.Unix(1, 0),
		), ShouldBeNil)
		thesis.Forecasts = []types.Forecasts{
			strategyForecast("BTC/USD", -0.01),
			strategyForecast("ETH/USD", 0.02),
		}

		intents := planner.Decide(
			thesis,
			map[string]types.Exposure{"BTC/USD": {
				Thesis: positionThesis, Quantity: 1, Mark: 100, Notional: 100,
			}},
			map[string]float64{"BTC/USD": 0.002, "ETH/USD": 0.002},
			100,
			2,
		)

		Convey("Then exit and entry are independently selected from current utility", func() {
			So(positionThesis.Decisions, ShouldHaveLength, 1)
			So(positionThesis.Decisions[0].Action, ShouldEqual, "exit")
			So(positionThesis.Forecasts, ShouldHaveLength, 1)
			So(thesis.Decisions, ShouldHaveLength, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "enter")
			So(thesis.Decisions[0].ProposedNotional, ShouldEqual, 100.0)
			So(thesis.Decisions[0].ExpectedFees, ShouldEqual, 0.004)
			So(thesis.Decisions[0].AvailableCapital, ShouldEqual, 100.0)
			So(thesis.Decisions[0].OpenPositions, ShouldEqual, 1)
			So(thesis.Decisions[0].SlotCapacity, ShouldEqual, 2)
			So(intents, ShouldHaveLength, 2)
			So(intents[0].Thesis, ShouldEqual, positionThesis)
			So(intents[0].Selected(), ShouldResemble, positionThesis.Decisions[0])
			So(intents[1].Thesis, ShouldEqual, thesis)
			So(intents[1].Selected(), ShouldResemble, thesis.Decisions[0])
		})
	})
}

func TestPlannerDecideRecordsNothing(t *testing.T) {
	Convey("Given an eligible forecast whose executable utility is negative", t, func() {
		planner := NewPlanner(context.Background(), nil, nil, nil)
		thesis := types.NewThesis(nil)
		thesis.Forecasts = []types.Forecasts{strategyForecast("ETH/USD", -0.01)}

		intents := planner.Decide(
			thesis,
			map[string]types.Exposure{},
			map[string]float64{"ETH/USD": 0.002},
			100,
			2,
		)

		Convey("Then doing nothing is retained without producing a broker command", func() {
			So(intents, ShouldBeEmpty)
			So(thesis.Decisions, ShouldHaveLength, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "nothing")
			So(thesis.Decisions[0].Utility, ShouldEqual, 0.0)
			So(thesis.Decisions[0].Alternatives, ShouldContainKey, "enter")
			So(thesis.LifecycleState("ETH/USD"), ShouldEqual, types.LifecycleShaped)
		})
	})
}

func TestPlannerDecideReducesToVisibleCapacity(t *testing.T) {
	Convey("Given downside and insufficient visible bid capacity for a full exit", t, func() {
		planner := NewPlanner(context.Background(), nil, nil, nil)
		current := types.NewThesis(nil)
		current.Forecasts = []types.Forecasts{strategyForecast("BTC/USD", -0.02)}
		current.Forecasts[0].SellCapacity = 100
		lifecycle := types.NewThesis(nil)
		So(lifecycle.Transition(
			"BTC/USD", types.LifecycleManaging, time.Unix(1, 0),
		), ShouldBeNil)

		intents := planner.Decide(
			current,
			map[string]types.Exposure{"BTC/USD": {
				Thesis: lifecycle, Quantity: 1, Mark: 1000, Notional: 1000,
			}},
			map[string]float64{"BTC/USD": 0.002},
			0,
			1,
		)

		Convey("Then reduction is sized from the executable fraction of the position", func() {
			So(intents, ShouldHaveLength, 1)
			So(intents[0].Selected().Action, ShouldEqual, "reduce")
			So(intents[0].Selected().ProposedQuantity, ShouldEqual, 0.1)
			So(intents[0].Selected().Alternatives, ShouldContainKey, "reduce")
			So(intents[0].Selected().Alternatives, ShouldNotContainKey, "exit")
			So(lifecycle.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleManaging)
		})
	})
}

/*
TestPlannerDecideRejectsInvalidRecovery verifies that strategy cannot invent
continuation reasoning for a holding whose originating Thesis was lost.
*/
func TestPlannerDecideRejectsInvalidRecovery(t *testing.T) {
	Convey("Given an orphan position with no recoverable Thesis", t, func() {
		planner := NewPlanner(context.Background(), nil, nil, nil)
		current := types.NewThesis(nil)
		current.Forecasts = []types.Forecasts{strategyForecast("BTC/USD", -0.02)}
		current.Forecasts[0].SellCapacity = 10
		orphan := types.NewThesis(nil)
		So(orphan.Transition(
			"BTC/USD", types.LifecycleInvalid, time.Unix(1, 0),
		), ShouldBeNil)

		intents := planner.Decide(
			current,
			map[string]types.Exposure{"BTC/USD": {
				Thesis: orphan, Quantity: 1, Mark: 1000, Notional: 1000,
			}},
			map[string]float64{"BTC/USD": 0.002},
			0,
			1,
		)

		Convey("Then no hold, reduction, or exit is manufactured", func() {
			So(intents, ShouldBeEmpty)
			So(orphan.Decisions, ShouldBeEmpty)
			So(orphan.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleInvalid)
		})
	})
}

func TestPlannerDecideDistinguishesExitCauses(t *testing.T) {
	Convey("Given separate weakening, expiry, and opposing-hypothesis lifecycles", t, func() {
		planner := NewPlanner(context.Background(), nil, nil, nil)
		forecast := strategyForecast("BTC/USD", -0.01)

		Convey("Current negative utility without stronger evidence is weakening", func() {
			lifecycle := types.NewThesis(nil)
			So(lifecycle.Transition(
				"BTC/USD", types.LifecycleManaging, time.Unix(1, 0),
			), ShouldBeNil)

			decision := planner.continuation(forecast, 0.002, types.Exposure{
				Thesis: lifecycle, Quantity: 1, Mark: 100, Notional: 100,
			})
			decision.Cause = planner.cause(lifecycle, forecast, decision.Action)

			So(decision.Action, ShouldEqual, "exit")
			So(decision.Cause, ShouldEqual, "thesis_weakening")
		})

		Convey("An elapsed entry forecast is explicit invalidation", func() {
			lifecycle := types.NewThesis(nil)
			lifecycle.Decisions = append(lifecycle.Decisions, types.Decision{
				Action: "enter", Symbol: "BTC/USD", ValidThroughEpoch: forecast.SourceEpoch,
			})

			So(planner.cause(lifecycle, forecast, "exit"),
				ShouldEqual, "thesis_invalidation")
		})

		Convey("A mature negative causal outcome is opposing-thesis formation", func() {
			lifecycle := types.NewThesis(nil)
			lifecycle.Hypotheses = append(lifecycle.Hypotheses, types.Hypothesis{
				Symbol: "BTC/USD", Ready: true, Outcome: forecast.Target,
				DoExpectation: -0.01, Uplift: -0.005,
			})

			So(planner.cause(lifecycle, forecast, "exit"),
				ShouldEqual, "opposing_thesis")
		})
	})
}

func BenchmarkPlannerDecide(b *testing.B) {
	planner := NewPlanner(context.Background(), nil, nil, nil)

	for b.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Forecasts = []types.Forecasts{strategyForecast("ETH/USD", 0.02)}
		intents := planner.Decide(
			thesis,
			map[string]types.Exposure{},
			map[string]float64{"ETH/USD": 0.002},
			100,
			2,
		)

		if len(intents) != 1 {
			b.Fatal("entry intent not selected")
		}
	}
}

func strategyForecast(symbol string, expectedReturn float64) types.Forecasts {
	return types.Forecasts{
		Source: "manifold_forecast", Symbol: symbol, At: time.Unix(1, 0),
		SourceEpoch: 1, HorizonEvents: 1, ExpiresEpoch: 2,
		Target: "next_l3_epoch_mid_log_return", ModelVersion: "test",
		Ready: true, Calibrated: true, FrictionReady: true,
		ExpectedReturn: expectedReturn, ExpectedSpread: 0.001,
		ReferencePrice: 100, BuyCapacity: 100,
		SellCapacity: 100,
	}
}
