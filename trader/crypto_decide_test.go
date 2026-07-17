package trader

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
TestCryptoDecideAppliesFriction ensures Tick-path Decide receives fee-ready
forecasts instead of silently skipping every Eligible gate.
*/
func TestCryptoDecideAppliesFriction(t *testing.T) {
	Convey("Given a forecast missing friction and a cached taker fee", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)

		crypto := &Crypto{
			price:   price,
			desk:    broker.NewDesk(nil, nil, nil, nil),
			planner: strategy.NewPlanner(context.Background(), nil, nil, nil),
		}

		thesis := types.NewThesis(nil, nil)
		thesis.Forecasts = append(thesis.Forecasts, eligibleForecastWithoutFriction())
		crypto.decide(thesis)

		Convey("Then friction is applied and Decide emits an explicit action", func() {
			So(thesis.Forecasts[0].FrictionReady, ShouldBeTrue)
			So(thesis.Forecasts[0].ExpectedFees, ShouldEqual, 0.0026)
			So(len(thesis.Decisions), ShouldEqual, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "nothing")
			So(thesis.Decisions[0].Cause, ShouldEqual, "cognitive_not_ready")
		})
	})
}

/*
TestCryptoApplyFrictionLeavesForecastUntouchedWithoutPrice ensures missing
broker fees do not invent FrictionReady and silently greenlight Eligible.
*/
func TestCryptoApplyFrictionLeavesForecastUntouchedWithoutPrice(t *testing.T) {
	Convey("Given forecasts and no Price surface", t, func() {
		crypto := &Crypto{}
		thesis := types.NewThesis(nil, nil)
		thesis.Forecasts = append(thesis.Forecasts, eligibleForecastWithoutFriction())

		fees := crypto.applyFriction(thesis)

		Convey("Then friction stays unset", func() {
			So(fees, ShouldBeEmpty)
			So(thesis.Forecasts[0].FrictionReady, ShouldBeFalse)
			So(thesis.Forecasts[0].ExpectedFees, ShouldEqual, 0)
		})
	})
}

func eligibleForecastWithoutFriction() types.Forecasts {
	return types.Forecasts{
		Source:                   "resonance+causal",
		Symbol:                   "BTC/USD",
		At:                       time.Unix(1, 0).UTC(),
		ObservedInterval:         time.Second,
		SourceEpoch:              1,
		HorizonEvents:            1,
		ExpiresEpoch:             2,
		Target:                   "next_l3_epoch_mid_log_return",
		ModelVersion:             "resonance_return_head_v1",
		Ready:                    true,
		Calibrated:               true,
		CalibrationSamples:       8,
		ExpectedReturn:           0.01,
		ReferencePrice:           100,
		BuyCapacity:              10,
		SellCapacity:             10,
		ExpectedSpread:           0.001,
		ExpectedImpact:           0,
		ExpectedAdverseSelection: 0,
		Uncertainty:              0.01,
		Confidence:               0.8,
	}
}
