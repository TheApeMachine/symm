package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestContinuationManageEmitsHoldForOccupiedForecast(t *testing.T) {
	Convey("Given an open lot whose forecast only lacks FrictionReady", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		price := broker.NewPrice(nil)
		So(price.RememberFee("PENGU/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)

		balance := broker.NewBalance(nil, nil, nil)
		lot := &types.Holding{
			Symbol: "PENGU/USD",
			Mark:   decimal.NewFromFloat64(0.006),
			Qty:    decimal.NewFromFloat64(1000),
			Status: types.OPEN,
		}
		balance.Seed(lot)

		desk := broker.NewDesk(nil, nil, price, balance)
		thesis := types.NewThesis(nil, nil)
		thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
			Source:             "resonance+causal",
			Symbol:             "PENGU/USD",
			At:                 time.Unix(1, 0).UTC(),
			Target:             "next_l3_epoch_mid_log_return",
			ModelVersion:       "resonance_return_head_v2_rls",
			Ready:              true,
			Calibrated:         true,
			SourceEpoch:        1,
			HorizonEvents:      1,
			ExpiresEpoch:       2,
			ExpectedReturn:     0.02,
			ReferencePrice:     decimal.NewFromFloat64(0.006),
			BuyCapacity:        decimal.NewFromInt64(100),
			SellCapacity:       decimal.NewFromInt64(100),
			ExpectedSpread:     0.001,
			ExpectedImpact:     0.0001,
			Uncertainty:        0.01,
			Confidence:         0.5,
			IncrementalMSE:     0.01,
			CalibrationSamples: 8,
		})

		opportunity := NewOpportunity(ctx, cancel, price, balance, nil, nil)
		opportunity.StampFriction(thesis)
		NewContinuity(price, balance, desk, NewRotate(), NewEvidence()).Manage(thesis)

		Convey("Then Continuity publishes a hold decision for the open symbol", func() {
			So(thesis.Forecasts[0].FrictionReady, ShouldBeTrue)
			So(len(thesis.Decisions), ShouldEqual, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, types.ActionHold)
			So(thesis.Decisions[0].Symbol, ShouldEqual, "PENGU/USD")
			So(thesis.Decisions[0].Cause, ShouldEqual, "continuation")
		})
	})
}

func TestContinuationManageAwaitsMissingForecast(t *testing.T) {
	Convey("Given an open lot without a forecast", t, func() {
		price := broker.NewPrice(nil)
		balance := broker.NewBalance(nil, nil, nil)
		balance.Seed(&types.Holding{
			Symbol: "PENGU/USD",
			Mark:   decimal.NewFromFloat64(0.006),
			Qty:    decimal.NewFromFloat64(1000),
			Status: types.OPEN,
		})
		desk := broker.NewDesk(nil, nil, price, balance)
		thesis := types.NewThesis(nil, nil)

		NewContinuity(price, balance, desk, NewRotate(), NewEvidence()).Manage(thesis)

		Convey("Then Continuity still publishes an awaiting-hold decision", func() {
			So(len(thesis.Decisions), ShouldEqual, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, types.ActionHold)
			So(thesis.Decisions[0].Reason, ShouldContainSubstring, "awaiting eligible forecast")
		})
	})
}
