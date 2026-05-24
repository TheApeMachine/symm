package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestForecastFromEvaluations(t *testing.T) {
	convey.Convey("Given scored evaluation rows", t, func() {
		snapshot := forecastFromEvaluations(0.9, []map[string]any{
			{"expected_return": 0.8},
			{"expected_return": 1.2},
		})

		convey.Convey("It should average expected return and line error", func() {
			convey.So(snapshot.PredictedSymbols, convey.ShouldEqual, 2)
			convey.So(snapshot.AvgPrediction, convey.ShouldAlmostEqual, 1.0, 1e-9)
			convey.So(snapshot.AvgError, convey.ShouldAlmostEqual, 0.1, 1e-9)
		})
	})
}

func TestAggregateForecasts(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)

	crypto := testCrypto(t, stubPrices{"PUMP/EUR": 101, "DUMP/EUR": 49}, &stubSignal{})

	pumpState := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})
	dumpState := crypto.pairState(asset.Pair{Wsname: "DUMP/EUR"})

	pumpMeasurement := testMeasurement(0.5)
	dumpMeasurement := engine.Measurement{
		Source:     "hawkes",
		Type:       engine.Momentum,
		Regime:     "momentum",
		Reason:     "cluster_sell",
		Confidence: 0.5,
	}
	pumpForecast := testForecast(0.002, time.Second)
	dumpForecast := testForecast(0.004, time.Second)

	pumpState.Update(pumpMeasurement)
	pumpState.ApplyForecast(pumpForecast)
	pumpState.RecordPrediction(start, pumpMeasurement, pumpForecast, 100)

	dumpState.Update(dumpMeasurement)
	dumpState.ApplyForecast(dumpForecast)
	dumpState.RecordPrediction(start, dumpMeasurement, dumpForecast, 50)

	convey.Convey("Given anchored forecasts on multiple symbols", t, func() {
		snapshot := crypto.aggregateForecasts()

		convey.Convey("It should average prediction and error across symbols", func() {
			convey.So(snapshot.PredictedSymbols, convey.ShouldEqual, 2)
			convey.So(snapshot.ErrorSymbols, convey.ShouldEqual, 2)
			convey.So(snapshot.AvgPrediction, convey.ShouldAlmostEqual, 0.003, 1e-9)
			convey.So(snapshot.AvgError, convey.ShouldAlmostEqual, 0.008, 1e-9)
		})
	})
}

func TestResolveForecastPrefersPairStates(t *testing.T) {
	crypto := testCrypto(t, stubPrices{"PUMP/EUR": 101}, &stubSignal{})

	state := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})
	measurement := testMeasurement(0.5)
	forecast := testForecast(0.002, time.Second)

	state.Update(measurement)
	state.ApplyForecast(forecast)
	state.RecordPrediction(time.Unix(1_700_000_000, 0), measurement, forecast, 100)

	readings := map[string]symbolReadings{
		"PUMP/EUR": {
			"hawkes": {
				source:         "hawkes",
				confidence:     0.8,
				expectedReturn: 0.002,
			},
		},
	}

	snapshot := crypto.resolveForecast(readings, 0.9, []map[string]any{
		{"combined": 0.5},
	})

	if snapshot.PredictedSymbols != 1 {
		t.Fatalf("expected pair-state forecast, got %+v", snapshot)
	}

	if snapshot.AvgPrediction != 0.002 {
		t.Fatalf("expected expected-return average, got %v", snapshot.AvgPrediction)
	}
}
