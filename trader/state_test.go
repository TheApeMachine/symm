package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func testForecast(expectedReturn float64, runway time.Duration) SignalForecast {
	return SignalForecast{
		ExpectedReturn: expectedReturn,
		Runway:         runway,
	}
}

func signalMeasurement(source string) engine.Measurement {
	return engine.Measurement{
		Source:     source,
		Type:       engine.Momentum,
		Regime:     "momentum",
		Reason:     "cluster_buy",
		Confidence: 0.5,
	}
}

func TestUpdate(t *testing.T) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}

	convey.Convey("Given a signal reading and trader forecast", t, func() {
		state := NewPairState(pair)
		state.Update(signalMeasurement("hawkes"))
		state.ApplyForecast(testForecast(0.002, 10*time.Second))

		convey.Convey("It should store confidence separately from expected return", func() {
			score, runway := state.Predict()

			convey.So(runway, convey.ShouldEqual, 10*time.Second)
			convey.So(score, convey.ShouldAlmostEqual, 0.002, 0.0000001)
		})
	})
}

func TestPredict(t *testing.T) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}

	convey.Convey("Given zero expected return", t, func() {
		state := NewPairState(pair)

		convey.Convey("It should return zero score and runway", func() {
			score, runway := state.Predict()

			convey.So(score, convey.ShouldEqual, 0)
			convey.So(runway, convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given expected return without a runway estimate", t, func() {
		state := NewPairState(pair)
		state.ApplyForecast(testForecast(0.002, 0))

		convey.Convey("It should not invent a horizon", func() {
			score, runway := state.Predict()

			convey.So(score, convey.ShouldEqual, 0)
			convey.So(runway, convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given a trader-derived runway", t, func() {
		state := NewPairState(pair)
		state.ApplyForecast(testForecast(0.012, 12*time.Second))

		convey.Convey("It should return expected return for ranking", func() {
			score, runway := state.Predict()

			convey.So(runway, convey.ShouldEqual, 12*time.Second)
			convey.So(score, convey.ShouldAlmostEqual, 0.012, 0.0001)
		})
	})

	convey.Convey("Given two readings with different expected returns", t, func() {
		weak := NewPairState(pair)
		weak.ApplyForecast(testForecast(0.003, 10*time.Second))

		strong := NewPairState(pair)
		strong.ApplyForecast(testForecast(0.009, 10*time.Second))

		convey.Convey("It should rank the stronger expected return higher", func() {
			weakScore, _ := weak.Predict()
			strongScore, _ := strong.Predict()

			convey.So(strongScore, convey.ShouldBeGreaterThan, weakScore)
		})
	})
}

func TestRecordPrediction(t *testing.T) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}
	now := time.Unix(1_700_000_000, 0)
	measurement := signalMeasurement("hawkes")
	forecast := testForecast(0.002, 10*time.Second)

	convey.Convey("Given a valid forecast", t, func() {
		state := NewPairState(pair)

		convey.Convey("It should store the trader forecast", func() {
			recorded := state.RecordPrediction(now, measurement, forecast, 100)

			convey.So(recorded, convey.ShouldBeTrue)
			convey.So(state.PendingCount(), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given no expected return", t, func() {
		state := NewPairState(pair)

		convey.Convey("It should not store a prediction", func() {
			convey.So(
				state.RecordPrediction(now, measurement, testForecast(0, 10*time.Second), 100),
				convey.ShouldBeFalse,
			)
		})
	})

	convey.Convey("Given an open forecast for the same source", t, func() {
		state := NewPairState(pair)
		state.RecordPrediction(now, measurement, forecast, 100)
		replacement := testForecast(0.003, 10*time.Second)

		convey.Convey("It should replace the open forecast", func() {
			state.RecordPrediction(now.Add(time.Second), measurement, replacement, 100)

			convey.So(state.PendingCount(), convey.ShouldEqual, 1)
		})
	})
}

func TestAnchorPending(t *testing.T) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}
	now := time.Unix(1_700_000_000, 0)

	convey.Convey("Given unanchored forecasts", t, func() {
		state := NewPairState(pair)
		state.RecordPrediction(now, signalMeasurement("hawkes"), testForecast(0.002, time.Second), 0)
		state.RecordPrediction(now, signalMeasurement("fluid"), testForecast(0.002, time.Second), 0)

		convey.Convey("It should anchor every pending forecast", func() {
			state.AnchorPending(100)

			convey.So(state.predictions[0].baselineQuote, convey.ShouldEqual, 100)
			convey.So(state.predictions[1].baselineQuote, convey.ShouldEqual, 100)
		})
	})
}

func TestSettleDue(t *testing.T) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}
	start := time.Unix(1_700_000_000, 0)
	measurement := signalMeasurement("hawkes")
	forecast := testForecast(0.001, 5*time.Second)

	convey.Convey("Given a matured anchored prediction", t, func() {
		state := NewPairState(pair)
		state.RecordPrediction(start, measurement, forecast, 100)

		convey.Convey("It should emit signed prediction feedback", func() {
			feedback := state.SettleDue(start.Add(5*time.Second), 110)

			convey.So(len(feedback), convey.ShouldEqual, 1)
			convey.So(feedback[0].PredictedReturn, convey.ShouldEqual, 0.001)
			convey.So(feedback[0].ActualReturn, convey.ShouldAlmostEqual, 0.1, 0.0001)
			convey.So(feedback[0].Regime, convey.ShouldEqual, "momentum")
			convey.So(state.PendingCount(), convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given an immature prediction", t, func() {
		state := NewPairState(pair)
		state.RecordPrediction(start, measurement, forecast, 100)

		convey.Convey("It should keep the prediction pending", func() {
			feedback := state.SettleDue(start.Add(2*time.Second), 110)

			convey.So(len(feedback), convey.ShouldEqual, 0)
			convey.So(state.PendingCount(), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given a matured prediction without a baseline quote", t, func() {
		state := NewPairState(pair)
		state.RecordPrediction(start, measurement, forecast, 0)

		convey.Convey("It should emit unanchored feedback and drop the forecast", func() {
			feedback := state.SettleDue(start.Add(5*time.Second), 110)

			convey.So(len(feedback), convey.ShouldEqual, 1)
			convey.So(feedback[0].Unanchored, convey.ShouldBeTrue)
			convey.So(state.PendingCount(), convey.ShouldEqual, 0)
		})
	})
}

func TestForecastMetrics(t *testing.T) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}
	now := time.Unix(1_700_000_000, 0)

	convey.Convey("Given no expected return", t, func() {
		state := NewPairState(pair)

		_, _, hasPrediction, hasError := state.ForecastMetrics(100)

		convey.Convey("It should report no forecast metrics", func() {
			convey.So(hasPrediction, convey.ShouldBeFalse)
			convey.So(hasError, convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given an unanchored forecast", t, func() {
		state := NewPairState(pair)
		state.ApplyForecast(testForecast(0.002, time.Second))
		state.RecordPrediction(now, signalMeasurement("hawkes"), testForecast(0.002, time.Second), 0)

		prediction, runningError, hasPrediction, hasError := state.ForecastMetrics(100)

		convey.Convey("It should expose prediction without error", func() {
			convey.So(hasPrediction, convey.ShouldBeTrue)
			convey.So(hasError, convey.ShouldBeFalse)
			convey.So(prediction, convey.ShouldEqual, 0.002)
			convey.So(runningError, convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given an anchored forecast", t, func() {
		state := NewPairState(pair)
		state.ApplyForecast(testForecast(0.002, time.Second))
		state.RecordPrediction(now, signalMeasurement("hawkes"), testForecast(0.002, time.Second), 100)
		state.AnchorPending(100)

		_, runningError, hasPrediction, hasError := state.ForecastMetrics(101)

		convey.Convey("It should expose running error against the live quote", func() {
			convey.So(hasPrediction, convey.ShouldBeTrue)
			convey.So(hasError, convey.ShouldBeTrue)
			convey.So(runningError, convey.ShouldAlmostEqual, -0.008, 1e-9)
		})
	})
}

func TestHasPendingPredictions(t *testing.T) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}
	now := time.Unix(1_700_000_000, 0)

	convey.Convey("Given no stored forecasts", t, func() {
		state := NewPairState(pair)

		convey.Convey("It should report no pending predictions", func() {
			convey.So(state.HasPendingPredictions(), convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given one stored forecast", t, func() {
		state := NewPairState(pair)
		state.RecordPrediction(now, signalMeasurement("hawkes"), testForecast(0.001, time.Second), 100)

		convey.Convey("It should report pending predictions", func() {
			convey.So(state.HasPendingPredictions(), convey.ShouldBeTrue)
			convey.So(state.PendingCount(), convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkPredict(b *testing.B) {
	state := NewPairState(asset.Pair{Wsname: "PUMP/EUR"})
	state.ApplyForecast(testForecast(0.004, 500*time.Millisecond))

	b.ReportAllocs()

	for b.Loop() {
		state.Predict()
	}
}

func BenchmarkSettleDue(b *testing.B) {
	pair := asset.Pair{Wsname: "PUMP/EUR"}
	start := time.Unix(1_700_000_000, 0)
	dueAt := start.Add(time.Millisecond)
	measurement := signalMeasurement("hawkes")
	forecast := testForecast(0.001, time.Millisecond)

	b.ReportAllocs()

	for b.Loop() {
		state := NewPairState(pair)
		state.RecordPrediction(start, measurement, forecast, 100)
		state.SettleDue(dueAt, 101)
	}
}
