package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestContinuationUtilityIsKeepScore(t *testing.T) {
	Convey("Given a managing forecast with uncertainty", t, func() {
		planner := &Planner{}
		forecast := types.Forecasts{
			Symbol:         "IDEX/USD",
			At:             time.Unix(1, 0).UTC(),
			ExpectedReturn: -0.2142,
			Uncertainty:    0.05,
			Confidence:     0.42,
			ExpectedSpread: 0.002,
			ExpectedImpact: 0.001,
			SellCapacity:   1e9,
			ReferencePrice: 0.01,
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

		decision := planner.continuation(forecast, 0.0026, holding)

		Convey("It should publish keep-score utility, not raw return", func() {
			So(decision.Action, ShouldEqual, types.ActionHold)
			So(decision.Utility, ShouldEqual, -0.2642)
			So(decision.Utility, ShouldNotEqual, forecast.ExpectedReturn)
			So(decision.ExpectedReturn.Float64(), ShouldEqual, -0.2142)
			So(decision.Confidence, ShouldEqual, 0.42)
			So(decision.Uncertainty, ShouldEqual, 0.05)
			So(decision.ProposedNotional.Float64(), ShouldEqual, 0)
		})
	})
}
