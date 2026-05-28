package price

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func TestPredictionSettlesFeedback(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	feedback := pool.CreateBroadcastGroup("feedback", 10*time.Millisecond)
	subscriber := feedback.Subscribe("test:feedback", 8)

	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)
	prediction.SeedReturnCalibration(source, "PUMP/EUR", 0.01)
	prediction.prices["PUMP/EUR"] = 1.02
	prediction.open["PUMP/EUR"] = map[string]openPrediction{
		source: {
			perspective: engine.Perspective{Type: engine.PerspectiveMicrostructure},
			measurement: engine.Measurement{
				Last:       100,
				Source:     "pumpdump",
				Type:       engine.Pump,
				Regime:     "microstructure",
				Reason:     "actual_pump",
				Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
				Confidence: 0.8,
			},
			source:          source,
			sources:         []string{"pumpdump"},
			predictedReturn: 0.008,
			confidence:      0.8,
			anchorPrice:     1.0,
			direction:       1,
			runway:          config.System.ScalpHoldBeforeExit,
			dueAt:           time.Now().Add(-time.Millisecond),
			predictedAt:     time.Now().Add(-config.System.ScalpHoldBeforeExit),
		},
	}

	go func() {
		_ = prediction.Tick()
	}()
	t.Cleanup(func() { _ = prediction.Close() })

	pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "PUMP/EUR", Last: 1.02},
	})

	select {
	case value := <-subscriber.Incoming:
		payload, ok := value.Value.(engine.PredictionFeedback)

		if !ok {
			t.Fatalf("expected prediction feedback, got %T", value.Value)
		}

		if payload.Source != source || payload.Symbol != "PUMP/EUR" {
			t.Fatalf("unexpected feedback: %+v", payload)
		}

		if !engine.FeedbackIncludesSource(payload, "pumpdump") {
			t.Fatalf("expected feedback to include pumpdump source: %+v", payload)
		}

		if payload.PredictedReturn <= 0 || payload.ActualReturn <= 0 {
			t.Fatalf("expected positive returns, got predicted=%v actual=%v", payload.PredictedReturn, payload.ActualReturn)
		}

		if payload.DueAt.IsZero() || payload.PredictedAt.IsZero() || payload.SettledAt.IsZero() {
			t.Fatalf("expected feedback timing fields, got %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("expected feedback on settle")
	}
}

func TestPredictionRunningMeanError(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	prediction.errorSum = 0.6
	prediction.errorCount = 3

	if got := prediction.RunningMeanError(); math.Abs(got-0.2) > 1e-9 {
		t.Fatalf("expected 0.2, got %v", got)
	}
}

func TestPredictionLastPrice(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	prediction.prices["BTC/EUR"] = 50000

	if got := prediction.LastPrice("BTC/EUR"); got != 50000 {
		t.Fatalf("expected 50000, got %v", got)
	}

	if got := prediction.LastPrice("MISSING/EUR"); got != 0 {
		t.Fatalf("expected 0 for unknown symbol, got %v", got)
	}
}

func TestPredictionRecordPerspective(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)
	regime := "microstructure"

	for range MinForwardSamples {
		prediction.returnModel.Observe(source, regime, 0.8, 0.012)
	}

	measurement := engine.Measurement{
		Source:     "pumpdump",
		Type:       engine.Pump,
		Regime:     regime,
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.8,
		Last:       50000,
	}

	predicted := prediction.RecordPerspective(
		"BTC/EUR",
		testPerspective(measurement),
		time.Now(),
	)

	if predicted <= 0 {
		t.Fatalf("expected learned predicted return, got %v", predicted)
	}

	if prediction.open["BTC/EUR"][source].predictedReturn != predicted {
		t.Fatalf("expected open prediction stored")
	}
}

func TestPredictionRecordPerspectiveUsesRegimeLocalReturnSupport(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)

	for range MinForwardSamples {
		prediction.returnModel.Observe(source, "warmed", 0.8, 0.012)
	}

	measurement := engine.Measurement{
		Source:     "pumpdump",
		Type:       engine.Pump,
		Regime:     "cold",
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.8,
		Last:       50000,
	}

	predicted := prediction.RecordPerspective(
		"BTC/EUR",
		testPerspective(measurement),
		time.Now(),
	)

	if predicted != 0 {
		t.Fatalf("expected unrelated regime support not to forecast BTC/EUR, got %v", predicted)
	}
}

func TestPredictionRecordPerspectiveRecordsColdBucketWithZeroForecast(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	subscriber := ui.Subscribe("test:prediction-ui", 8)
	prediction.observeTicker(market.TickerRow{Symbol: "BTC/EUR", Last: 100})
	prediction.observeTicker(market.TickerRow{Symbol: "BTC/EUR", Last: 101})

	measurement := engine.Measurement{
		Source:     "hawkes",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.42,
		Last:       50000,
	}

	predicted := prediction.RecordPerspective(
		"BTC/EUR",
		testPerspective(measurement),
		time.Now(),
	)

	if predicted != 0 {
		t.Fatalf("expected cold return model forecast to be zero, got %v", predicted)
	}

	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)

	if _, ok := prediction.open["BTC/EUR"][source]; !ok {
		t.Fatal("expected cold bucket to still record an open prediction")
	}

	select {
	case value := <-subscriber.Incoming:
		row, ok := value.Value.(map[string]any)

		if !ok || row["event"] != "prediction" {
			t.Fatalf("expected prediction ui event, got %v", value.Value)
		}

		if row["due_at"] == nil || row["ts"] == nil {
			t.Fatalf("expected prediction timing fields, got %+v", row)
		}
	case <-time.After(time.Second):
		t.Fatal("expected prediction ui event")
	}
}

func TestPredictionRecordPerspectiveBlocksNegativeLearnedReturnSupport(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)

	for range MinForwardSamples {
		prediction.returnModel.Observe(source, "microstructure", 0.8, -0.012)
	}

	measurement := engine.Measurement{
		Source:     "hawkes",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.42,
		Last:       50000,
	}

	predicted := prediction.RecordPerspective(
		"BTC/EUR",
		testPerspective(measurement),
		time.Now(),
	)

	if predicted != 0 {
		t.Fatalf("expected negative return model bucket to block forecast, got %v", predicted)
	}
}

func TestPredictionRecordPerspectiveDoesNotInventScale(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)

	measurement := engine.Measurement{
		Source:     "hawkes",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.42,
		Last:       50000,
	}

	predicted := prediction.RecordPerspective(
		"BTC/EUR",
		testPerspective(measurement),
		time.Now(),
	)

	if predicted != 0 {
		t.Fatalf("expected no prediction without learned or market scale, got %v", predicted)
	}
}

func TestPredictionConcurrentRecordAndTick(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 4, 8, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)
	prediction.SeedReturnCalibration(source, "BTC/EUR", 0.01)
	prediction.prices["BTC/EUR"] = 50000
	prediction.open["BTC/EUR"] = map[string]openPrediction{
		source: {
			perspective: engine.Perspective{Type: engine.PerspectiveMicrostructure},
			measurement: engine.Measurement{
				Last:       100,
				Source:     "pumpdump",
				Type:       engine.Pump,
				Regime:     "microstructure",
				Reason:     "actual_pump",
				Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
				Confidence: 0.8,
			},
			source:          source,
			sources:         []string{"pumpdump"},
			predictedReturn: 0.008,
			confidence:      0.8,
			anchorPrice:     49000,
			direction:       1,
			runway:          config.System.ScalpHoldBeforeExit,
			dueAt:           time.Now().Add(-time.Millisecond),
			predictedAt:     time.Now().Add(-config.System.ScalpHoldBeforeExit),
		},
	}

	measurement := engine.Measurement{
		Source:     "pumpdump",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.8,
		Last:       50000,
	}

	go func() {
		_ = prediction.Tick()
	}()
	t.Cleanup(func() { _ = prediction.Close() })

	var workers sync.WaitGroup

	for index := 0; index < 32; index++ {
		workers.Add(1)

		go func() {
			defer workers.Done()
			prediction.RecordPerspective(
				"BTC/EUR",
				testPerspective(measurement),
				time.Now(),
			)
		}()
	}

	workers.Wait()

	pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "BTC/EUR", Last: 50000},
	})
}

// TestRecordPerspectiveDoesNotOverwriteOpen proves that predictions live in
// time, not in cycles: a second RecordPerspective call for the same
// (symbol, source) while a forecast is still open does not replace it. The
// open forecast keeps its original anchor and dueAt, so the settler can
// evaluate the actual forward return when time has caught up.
func TestRecordPerspectiveDoesNotOverwriteOpen(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)

	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)
	prediction.SeedReturnCalibration(source, "BTC/EUR", 0.01)

	first := engine.Measurement{
		Source:     "pumpdump",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.8,
		Last:       1.0,
	}

	now := time.Now()
	predictedFirst := prediction.RecordPerspective("BTC/EUR", testPerspective(first), now)

	open, ok := prediction.open["BTC/EUR"][source]
	if !ok {
		t.Fatal("expected first record to open a forecast")
	}
	originalAnchor := open.anchorPrice
	originalDueAt := open.dueAt

	second := first
	second.Last = 1.02
	predictedSecond := prediction.RecordPerspective("BTC/EUR", testPerspective(second), now.Add(time.Millisecond))

	if predictedSecond != predictedFirst {
		t.Fatalf("expected no-op overwrite to return prior predictedReturn (%v), got %v",
			predictedFirst, predictedSecond)
	}

	stillOpen, ok := prediction.open["BTC/EUR"][source]
	if !ok {
		t.Fatal("expected forecast to remain open after second record")
	}

	if stillOpen.anchorPrice != originalAnchor {
		t.Fatalf("expected anchor preserved: original=%v, after=%v",
			originalAnchor, stillOpen.anchorPrice)
	}

	if !stillOpen.dueAt.Equal(originalDueAt) {
		t.Fatalf("expected dueAt preserved: original=%v, after=%v",
			originalDueAt, stillOpen.dueAt)
	}
}

func testPerspective(measurement engine.Measurement) engine.Perspective {
	return engine.Perspective{
		Type:         engine.PerspectiveMicrostructure,
		Measurements: []engine.Measurement{measurement},
	}
}
