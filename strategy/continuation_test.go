package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/types"
)

func TestContinuationUtilityIsKeepScore(t *testing.T) {
	Convey("Given a managing forecast with uncertainty", t, func() {
		continuity := NewContinuity(broker.NewPrice(nil), nil, NewRotate())
		forecast := types.Forecasts{
			Symbol:         "IDEX/USD",
			At:             time.Unix(1, 0).UTC(),
			ExpectedReturn: -0.2142,
			Uncertainty:    0.05,
			Confidence:     0.42,
			ExpectedSpread: 0.002,
			ExpectedImpact: 0.001,
			SellCapacity:   decimal.NewFromInt64(1_000_000_000),
			ReferencePrice: decimal.NewFromFloat64(0.01),
			Source:         "resonance+causal",
			SourceEpoch:    1,
			ExpiresEpoch:   2,
		}
		holding := &types.Holding{
			Symbol: "IDEX/USD",
			Mark:   decimal.NewFromFloat64(0.01),
			Qty:    decimal.NewFromFloat64(100),
			Status: types.OPEN,
		}

		decision := continuity.Score(forecast, 0.0026, holding)

		Convey("It should publish keep-score utility and never reduce", func() {
			So(decision.Action, ShouldEqual, types.ActionHold)
			So(decision.Utility, ShouldEqual, -0.2642)
			So(decision.Utility, ShouldEqual, decision.Alternatives["hold"])
			So(decision.Alternatives["exit"], ShouldEqual, -continuity.rotate.Exit(forecast, 0.0026))
			So(decision.ExpectedReturn.Float64(), ShouldEqual, -0.2142)
			So(decision.Confidence, ShouldEqual, 0.42)
			So(decision.Uncertainty, ShouldEqual, 0.05)
			So(decision.ProposedNotional.Sign(), ShouldEqual, 0)
			So(decision.ProposedQuantity.Sign(), ShouldEqual, 0)
		})
	})
}

func TestContinuity_Score(t *testing.T) {
	Convey("Given visible bid capacity below the open notional", t, func() {
		continuity := NewContinuity(broker.NewPrice(nil), nil, NewRotate())
		quantity, err := decimal.NewFromString("10.000")
		So(err, ShouldBeNil)
		holding := &types.Holding{
			Symbol: "EXACT/USD",
			Mark:   decimal.NewFromInt64(3),
			Qty:    quantity,
			Status: types.OPEN,
		}
		forecast := types.Forecasts{
			Symbol:         holding.Symbol,
			At:             time.Unix(1, 0).UTC(),
			ExpectedReturn: -1,
			Uncertainty:    0,
			SellCapacity:   decimal.NewFromInt64(10),
			ReferencePrice: decimal.NewFromInt64(3),
			Source:         "resonance+causal",
			SourceEpoch:    1,
			ExpiresEpoch:   2,
		}

		decision := continuity.Score(forecast, 0, holding)

		Convey("It still holds the full lot; Stoploss owns exit", func() {
			So(decision.Action, ShouldEqual, types.ActionHold)
			So(decision.ProposedQuantity.Sign(), ShouldEqual, 0)
			So(decision.ProposedNotional.Sign(), ShouldEqual, 0)
			So(decision.Reason, ShouldEqual, "stoploss owns full exit; continuation holds")
		})
	})
}

/*
TestContinuityManageExhaustionTax proves that Manage taxes hold utility by the
category graph's exhaustion lead share when the resident graph shows the current
category sequence precedes exhaustion regimes.
*/
func TestContinuityManageExhaustionTax(t *testing.T) {
	Convey("Given an open lot with a forecast and an exhaustion-lead graph", t, func() {
		at := time.Unix(1, 0).UTC()
		graph := category.NewGraph()
		thesis := types.NewThesis()

		// Two-cut sequence: VerticalIgnition alone, then VerticalIgnition + Exhaustion.
		// Graph records Leads: VerticalIgnition → Exhaustion, which is exhaustion-dominant.
		graph.Update(at, thesis, []types.Category{
			{Symbol: "SIM1/USD", Type: types.VerticalIgnition, Strength: 0.9, Freshness: 1},
		})
		graph.Update(at.Add(time.Second), thesis, []types.Category{
			{Symbol: "SIM1/USD", Type: types.VerticalIgnition, Strength: 0.9, Freshness: 1, Supporting: []string{"ignition"}},
			{Symbol: "SIM1/USD", Type: types.Exhaustion, Strength: 0.8, Freshness: 1, Supporting: []string{"exhaust"}},
		})
		thesis.Graphs.Store("categories", graph)

		forecast := types.Forecasts{
			Symbol:         "SIM1/USD",
			At:             at,
			ExpectedReturn: 0.1,
			Uncertainty:    0.01,
			SellCapacity:   decimal.NewFromInt64(1_000_000_000),
			ReferencePrice: decimal.NewFromFloat64(1.0),
			Source:         "resonance+causal",
			SourceEpoch:    1,
			ExpiresEpoch:   2,
		}
		thesis.Forecasts = []types.Forecasts{forecast}

		balance := broker.NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.StoreHolding(&types.Holding{
			Symbol: "SIM1/USD",
			Mark:   decimal.NewFromFloat64(1.0),
			Qty:    decimal.NewFromFloat64(100),
			Status: types.OPEN,
		})

		continuity := NewContinuity(broker.NewPrice(nil), balance, NewRotate())
		rawHold := NewRotate().Hold(forecast)

		Convey("When Manage applies the exhaustion lead tax", func() {
			continuity.Manage(thesis)

			var holdDecision *types.Decision

			for index := range thesis.Decisions {
				if thesis.Decisions[index].Symbol == "SIM1/USD" &&
					thesis.Decisions[index].Action == types.ActionHold {
					holdDecision = &thesis.Decisions[index]
					break
				}
			}

			Convey("It should reduce hold utility below the raw keep score", func() {
				So(holdDecision, ShouldNotBeNil)
				So(holdDecision.Utility, ShouldBeLessThan, rawHold)
				So(holdDecision.Alternatives["hold"], ShouldEqual, holdDecision.Utility)
			})
		})
	})
}
