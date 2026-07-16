package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

type plannerCutSignal struct {
	high    int
	cut     int
	seen    int
	measure func()
}

func (signal *plannerCutSignal) Capture(time.Time) error {
	signal.cut = signal.high
	return nil
}

func (signal *plannerCutSignal) Measure(thesis *types.Thesis) *types.Thesis {
	signal.seen = signal.cut

	if signal.measure != nil {
		signal.measure()
	}

	return thesis
}

func TestPlanner_UpdateCapturesInputsFirst(t *testing.T) {
	Convey("Given an observation arriving while an earlier signal measures", t, func() {
		later := &plannerCutSignal{}
		earlier := &plannerCutSignal{
			measure: func() {
				later.high++
			},
		}
		planner := NewPlanner(
			context.Background(),
			nil,
			[]types.Signal{earlier, later},
			nil,
		)

		planner.Update()

		Convey("It defers that observation until the next Thesis", func() {
			So(later.seen, ShouldEqual, 0)
			planner.Update()
			So(later.seen, ShouldEqual, 1)
		})
	})
}

func TestPlannerDecide(t *testing.T) {
	forecast := types.Forecasts{
		Source: "manifold", Symbol: "BTC/USD", At: time.Unix(1, 0),
		ObservedInterval: time.Second, SourceEpoch: 1, HorizonEvents: 1,
		ExpiresEpoch: 2, Target: "mid_log_return", ModelVersion: "online",
		Ready: true, Calibrated: true, FrictionReady: true,
		CalibrationSamples: 1, ExpectedReturn: 0.05, ReferencePrice: 100,
		BuyCapacity: 100, SellCapacity: 100, ExpectedSpread: 0.001,
		ExpectedImpact: 0.001, ExpectedAdverseSelection: 0.001,
		Confidence: 1,
	}
	planner := &Planner{}

	Convey("Given an eligible forecast without cognitive memory", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, forecast)

		planner.Decide(thesis, map[string]float64{"BTC/USD": 0.001}, 100, 1)

		Convey("It should record no action without creating a broker order", func() {
			So(thesis.Decisions, ShouldHaveLength, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "nothing")
			So(thesis.Decisions[0].Cause, ShouldEqual, "cognitive_not_ready")
			So(thesis.Orders, ShouldBeEmpty)
			So(thesis.Positions, ShouldHaveLength, 1)
			So(thesis.Positions[0].Symbol, ShouldEqual, "BTC/USD")
			So(thesis.Positions[0].Qty.Sign(), ShouldEqual, 0)
		})
	})

	Convey("Given ready DMT memory that supports a buy entry", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, forecast)
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Source: "dmt", Symbol: "BTC/USD", At: forecast.At,
			Ready: true, Winner: "buy",
		})

		planner.Decide(thesis, map[string]float64{"BTC/USD": 0.001}, 100, 1)

		Convey("It should retain the decision and emit the selected entry", func() {
			So(thesis.Decisions, ShouldHaveLength, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "enter")
			So(thesis.Orders, ShouldHaveLength, 1)
			So(thesis.Orders[0].Description.Type, ShouldEqual, "enter")
			So(thesis.Orders[0].Volume.Float64(), ShouldAlmostEqual,
				thesis.Decisions[0].ProposedQuantity, 1e-12)
			So(thesis.Orders[0].Volume.Float64(), ShouldBeLessThan,
				thesis.Decisions[0].ProposedNotional)
			So(thesis.Positions, ShouldHaveLength, 1)
			So(thesis.Positions[0].Qty.Sign(), ShouldEqual, 1)
		})
	})

	Convey("Given an open position without ready cognitive memory", t, func() {
		thesis := types.NewThesis(nil)
		exitForecast := forecast
		exitForecast.ExpectedReturn = -0.05
		thesis.Forecasts = append(thesis.Forecasts, exitForecast)
		thesis.Positions = append(thesis.Positions, types.Holding{
			Order: &spot.Order{Description: &spot.OrderDescription{Pair: "BTC/USD"}},
			Qty:   decimal.NewFromFloat64(0.5),
			Mark:  decimal.NewFromFloat64(100),
		})

		planner.Decide(thesis, map[string]float64{"BTC/USD": 0.001}, 100, 1)

		Convey("It should still manage and exit the existing exposure", func() {
			So(thesis.Decisions, ShouldHaveLength, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "exit")
			So(thesis.Orders, ShouldHaveLength, 1)
			So(thesis.Orders[0].Description.Type, ShouldEqual, "exit")
		})
	})
}

func BenchmarkPlannerDecide(b *testing.B) {
	planner := &Planner{}
	forecast := types.Forecasts{
		Source: "manifold", Symbol: "BTC/USD", At: time.Unix(1, 0),
		ObservedInterval: time.Second, SourceEpoch: 1, HorizonEvents: 1,
		ExpiresEpoch: 2, Target: "mid_log_return", ModelVersion: "online",
		Ready: true, Calibrated: true, FrictionReady: true,
		CalibrationSamples: 1, ExpectedReturn: 0.05, ReferencePrice: 100,
		BuyCapacity: 100, SellCapacity: 100, ExpectedSpread: 0.001,
		ExpectedImpact: 0.001, ExpectedAdverseSelection: 0.001,
		Confidence: 1,
	}
	fees := map[string]float64{"BTC/USD": 0.001}

	for b.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, forecast)
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Source: "dmt", Symbol: "BTC/USD", At: forecast.At,
			Ready: true, Winner: "buy",
		})
		planner.Decide(thesis, fees, 100, 1)
	}
}
