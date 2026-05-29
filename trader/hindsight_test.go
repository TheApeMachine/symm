package trader

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHindsightTrackerRecordSkip(t *testing.T) {
	Convey("Given a skipped prediction with a settlement key", t, func() {
		hindsightTracker := newHindsightTracker()
		predictedAt := time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC)
		dueAt := predictedAt.Add(time.Second)
		prediction := hindsightTestPrediction("BTC/EUR", predictedAt, dueAt)

		hindsightTracker.RecordSkip(
			prediction,
			"edge_below_threshold",
			map[string]any{
				"symbol":   "BTC/EUR",
				"friction": 0.002,
				"ts":       predictedAt.Add(100 * time.Millisecond).Format(time.RFC3339Nano),
			},
		)

		key, ok := hindsightKeyForPrediction(prediction)
		So(ok, ShouldBeTrue)

		hindsightTracker.mu.Lock()
		decision := hindsightTracker.skipped[key]
		hindsightTracker.mu.Unlock()

		Convey("It should retain the reason until feedback settles", func() {
			So(decision.reason, ShouldEqual, "edge_below_threshold")
			So(decision.fields["friction"], ShouldEqual, 0.002)
		})
	})
}

func TestHindsightTrackerSettle(t *testing.T) {
	Convey("Given a skipped prediction that later settles net-positive", t, func() {
		originalLogDir := config.System.LogDir
		config.System.LogDir = t.TempDir()
		t.Cleanup(func() { config.System.LogDir = originalLogDir })

		hindsightTracker := newHindsightTracker()
		predictedAt := time.Date(2026, 5, 29, 1, 1, 0, 0, time.UTC)
		dueAt := predictedAt.Add(time.Second)
		prediction := hindsightTestPrediction("ETH/EUR", predictedAt, dueAt)

		hindsightTracker.RecordSkip(
			prediction,
			"edge_below_threshold",
			map[string]any{
				"symbol":   "ETH/EUR",
				"friction": 0.002,
				"edge":     -0.001,
				"ts":       predictedAt.Add(100 * time.Millisecond).Format(time.RFC3339Nano),
			},
		)
		hindsightTracker.RecordSkip(
			prediction,
			"open_prediction_pending",
			map[string]any{
				"symbol":   "ETH/EUR",
				"friction": 0.002,
				"ts":       predictedAt.Add(200 * time.Millisecond).Format(time.RFC3339Nano),
			},
		)

		err := hindsightTracker.Settle(engine.PredictionFeedback{
			Source:          engine.PerspectiveSource(engine.PerspectiveMicrostructure),
			Sources:         []string{"hawkes"},
			Contributions:   map[string]float64{"hawkes": 1},
			Symbol:          "ETH/EUR",
			Regime:          "cluster",
			Confidence:      0.9,
			PredictedReturn: 0.001,
			ActualReturn:    0.01,
			Error:           -0.009,
			PredictedAt:     predictedAt,
			DueAt:           dueAt,
			SettledAt:       dueAt.Add(time.Millisecond),
		})
		So(err, ShouldBeNil)

		payload, err := os.ReadFile(hindsightTracker.writer.path)
		So(err, ShouldBeNil)

		row := make(map[string]any)
		err = json.Unmarshal([]byte(strings.TrimSpace(string(payload))), &row)
		So(err, ShouldBeNil)

		Convey("It should write a missed-opportunity hindsight row", func() {
			So(row["event"], ShouldEqual, "hindsight_missed_opportunity")
			So(row["symbol"], ShouldEqual, "ETH/EUR")
			So(row["reason"], ShouldEqual, "edge_below_threshold")
			So(row["last_reason"], ShouldEqual, "open_prediction_pending")
			So(row["actual_return"], ShouldAlmostEqual, 0.01)
			So(row["friction"], ShouldAlmostEqual, 0.002)
			So(row["missed_return"], ShouldAlmostEqual, 0.008)
			So(row["required_return"], ShouldAlmostEqual, 0.004)
			So(row["return_multiple"], ShouldAlmostEqual, 2.5)

			decision, ok := row["decision"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(decision["reason"], ShouldEqual, "edge_below_threshold")

			decisions, ok := row["decisions"].([]any)
			So(ok, ShouldBeTrue)
			So(decisions, ShouldHaveLength, 2)
		})
	})

	Convey("Given a skipped prediction that settles below recorded friction", t, func() {
		originalLogDir := config.System.LogDir
		config.System.LogDir = t.TempDir()
		t.Cleanup(func() { config.System.LogDir = originalLogDir })

		hindsightTracker := newHindsightTracker()
		predictedAt := time.Date(2026, 5, 29, 1, 2, 0, 0, time.UTC)
		dueAt := predictedAt.Add(time.Second)
		prediction := hindsightTestPrediction("BTC/EUR", predictedAt, dueAt)

		hindsightTracker.RecordSkip(
			prediction,
			"edge_below_threshold",
			map[string]any{
				"symbol":   "BTC/EUR",
				"friction": 0.01,
			},
		)

		err := hindsightTracker.Settle(engine.PredictionFeedback{
			Source:          engine.PerspectiveSource(engine.PerspectiveMicrostructure),
			Symbol:          "BTC/EUR",
			PredictedReturn: 0.001,
			ActualReturn:    0.005,
			Error:           -0.004,
			PredictedAt:     predictedAt,
			DueAt:           dueAt,
			SettledAt:       dueAt.Add(time.Millisecond),
		})
		So(err, ShouldBeNil)

		Convey("It should not create a hindsight file", func() {
			So(hindsightTracker.writer.path, ShouldEqual, "")
		})
	})

	Convey("Given a skipped prediction that settles net-positive but below the entry multiple", t, func() {
		originalLogDir := config.System.LogDir
		config.System.LogDir = t.TempDir()
		t.Cleanup(func() { config.System.LogDir = originalLogDir })

		hindsightTracker := newHindsightTracker()
		predictedAt := time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC)
		dueAt := predictedAt.Add(time.Second)
		prediction := hindsightTestPrediction("CVX/EUR", predictedAt, dueAt)

		hindsightTracker.RecordSkip(
			prediction,
			"forward_model_not_ready",
			map[string]any{
				"symbol":   "CVX/EUR",
				"friction": 0.016,
				"ts":       predictedAt.Add(100 * time.Millisecond).Format(time.RFC3339Nano),
			},
		)

		err := hindsightTracker.Settle(engine.PredictionFeedback{
			Source:          engine.PerspectiveSource(engine.PerspectiveMicrostructure),
			Symbol:          "CVX/EUR",
			PredictedReturn: 0,
			ActualReturn:    0.026,
			Error:           -0.026,
			PredictedAt:     predictedAt,
			DueAt:           dueAt,
			SettledAt:       dueAt.Add(time.Millisecond),
		})
		So(err, ShouldBeNil)

		Convey("It should not create a hindsight file", func() {
			So(hindsightTracker.writer.path, ShouldEqual, "")
		})
	})
}

func TestHindsightTrackerRecordSkipIgnoresPostDueDecisions(t *testing.T) {
	Convey("Given a skip recorded after the prediction due time", t, func() {
		hindsightTracker := newHindsightTracker()
		predictedAt := time.Date(2026, 5, 29, 1, 5, 0, 0, time.UTC)
		dueAt := predictedAt.Add(time.Second)
		prediction := hindsightTestPrediction("BTC/EUR", predictedAt, dueAt)

		hindsightTracker.RecordSkip(
			prediction,
			"forward_model_not_ready",
			map[string]any{
				"symbol":   "BTC/EUR",
				"friction": 0.002,
				"ts":       predictedAt.Add(100 * time.Millisecond).Format(time.RFC3339Nano),
			},
		)
		hindsightTracker.RecordSkip(
			prediction,
			"open_prediction_pending",
			map[string]any{
				"symbol":   "BTC/EUR",
				"friction": 0.002,
				"ts":       dueAt.Add(time.Millisecond).Format(time.RFC3339Nano),
			},
		)

		key, ok := hindsightKeyForPrediction(prediction)
		So(ok, ShouldBeTrue)

		hindsightTracker.mu.Lock()
		decision := hindsightTracker.skipped[key]
		hindsightTracker.mu.Unlock()

		Convey("It should keep only pre-due decisions", func() {
			So(decision.reason, ShouldEqual, "forward_model_not_ready")
			So(decision.lastReason, ShouldEqual, "forward_model_not_ready")
			So(decision.observations, ShouldHaveLength, 1)
		})
	})
}

func BenchmarkHindsightTrackerSettle(b *testing.B) {
	originalLogDir := config.System.LogDir
	config.System.LogDir = b.TempDir()
	b.Cleanup(func() { config.System.LogDir = originalLogDir })

	hindsightTracker := newHindsightTracker()
	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)
	baseAt := time.Date(2026, 5, 29, 1, 3, 0, 0, time.UTC)
	index := 0

	b.ReportAllocs()

	for b.Loop() {
		predictedAt := baseAt.Add(time.Duration(index) * time.Nanosecond)
		dueAt := predictedAt.Add(time.Second)
		prediction := hindsightTestPrediction("BTC/EUR", predictedAt, dueAt)

		hindsightTracker.RecordSkip(
			prediction,
			"edge_below_threshold",
			map[string]any{
				"symbol":   "BTC/EUR",
				"friction": 0.002,
			},
		)

		if err := hindsightTracker.Settle(engine.PredictionFeedback{
			Source:          source,
			Symbol:          "BTC/EUR",
			PredictedReturn: 0.001,
			ActualReturn:    0.01,
			Error:           -0.009,
			PredictedAt:     predictedAt,
			DueAt:           dueAt,
			SettledAt:       dueAt.Add(time.Millisecond),
		}); err != nil {
			b.Fatal(err)
		}

		index++
	}
}

func hindsightTestPrediction(
	symbol string,
	predictedAt time.Time,
	dueAt time.Time,
) engine.Prediction {
	measurement := engine.Measurement{
		Type:       engine.Momentum,
		Source:     "hawkes",
		Regime:     "cluster",
		Reason:     "burst",
		Pairs:      []asset.Pair{{Wsname: symbol}},
		Confidence: 0.9,
		Last:       100,
		Bid:        99.9,
		Ask:        100.1,
	}

	return engine.Prediction{
		Perspective: engine.Perspective{
			Type:         engine.PerspectiveMicrostructure,
			Measurements: []engine.Measurement{measurement},
		},
		Confidence:     0.9,
		ExpectedReturn: 0.001,
		PredictedAt:    predictedAt,
		DueAt:          dueAt,
		Runway:         dueAt.Sub(predictedAt),
	}
}
