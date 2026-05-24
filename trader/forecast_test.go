package trader

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
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
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	crypto, err := NewCrypto(
		context.Background(),
		nil,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 101, "DUMP/EUR": 49},
		&stubSignal{},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	pumpState := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})
	dumpState := crypto.pairState(asset.Pair{Wsname: "DUMP/EUR"})

	pumpState.Update(testMeasurement(0.002, time.Second))
	pumpState.RecordPrediction(start, testMeasurement(0.002, time.Second))
	pumpState.AnchorPending(100)

	dumpState.Update(testMeasurement(0.004, time.Second))
	dumpState.RecordPrediction(start, testMeasurement(0.004, time.Second))
	dumpState.AnchorPending(50)

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
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

	crypto, err := NewCrypto(
		context.Background(),
		nil,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 101},
		&stubSignal{},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	state := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})
	state.Update(testMeasurement(0.002, time.Second))
	state.RecordPrediction(time.Unix(1_700_000_000, 0), testMeasurement(0.002, time.Second))
	state.AnchorPending(100)

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
