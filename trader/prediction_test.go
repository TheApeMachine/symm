package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestBuildPrediction(t *testing.T) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}
	now := time.Unix(1_700_000_000, 0)

	convey.Convey("Given a valid trader forecast", t, func() {
		state := NewPairState(pair)
		measurement := engine.Measurement{
			Source:     "hawkes",
			Type:       engine.Momentum,
			Regime:     "momentum",
			Reason:     "cluster_buy",
			Confidence: 0.5,
		}
		forecast := testForecast(0.002, 10*time.Second)

		convey.Convey("It should build a due forecast", func() {
			prediction, ok := state.buildPrediction(now, measurement, forecast, 100)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(prediction.expectedReturn, convey.ShouldEqual, 0.002)
			convey.So(prediction.dueAt, convey.ShouldEqual, now.Add(10*time.Second))
		})
	})
}

func TestSignedActualReturn(t *testing.T) {
	prediction := Prediction{
		baselineQuote: 100,
		direction:     1,
	}

	convey.Convey("Given a buy-side move up", t, func() {
		convey.Convey("It should return a positive signed return", func() {
			convey.So(prediction.signedActualReturn(110), convey.ShouldAlmostEqual, 0.1, 0.0001)
		})
	})

	convey.Convey("Given a buy-side move down", t, func() {
		convey.Convey("It should return a negative signed return", func() {
			convey.So(prediction.signedActualReturn(90), convey.ShouldAlmostEqual, -0.1, 0.0001)
		})
	})
}

func TestSettle(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	prediction := Prediction{
		source:          "hawkes",
		symbol:          "PUMP/EUR",
		measurementType: engine.Momentum,
		regime:          "momentum",
		reason:          "cluster_buy",
		direction:       1,
		baselineQuote:   100,
		expectedReturn:  0.001,
		runway:          5 * time.Second,
	}

	convey.Convey("Given an anchored forecast", t, func() {
		convey.Convey("It should emit prediction feedback", func() {
			feedback := prediction.settle(110, start.Add(5*time.Second))

			convey.So(feedback.PredictedReturn, convey.ShouldEqual, 0.001)
			convey.So(feedback.ActualReturn, convey.ShouldAlmostEqual, 0.1, 0.0001)
			convey.So(feedback.Unanchored, convey.ShouldBeFalse)
		})
	})
}

func TestSettleUnanchored(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	prediction := Prediction{
		source:         "hawkes",
		symbol:         "PUMP/EUR",
		expectedReturn: 0.001,
		runway:         5 * time.Second,
	}

	convey.Convey("Given a forecast without a baseline quote", t, func() {
		convey.Convey("It should mark the feedback as unanchored", func() {
			feedback := prediction.settleUnanchored(start.Add(5 * time.Second))

			convey.So(feedback.Unanchored, convey.ShouldBeTrue)
			convey.So(feedback.ActualReturn, convey.ShouldEqual, 0)
		})
	})
}
