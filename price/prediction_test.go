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
)

func TestPredictionSettlesFeedback(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	feedback := pool.CreateBroadcastGroup("feedback", 10*time.Millisecond)
	subscriber := feedback.Subscribe("test:feedback", 8)

	prediction.SeedReturnCalibration("pumpdump", 0.01)
	prediction.prices["PUMP/EUR"] = 1.02
	prediction.open["PUMP/EUR"] = map[string]openPrediction{
		"pumpdump": {
			measurement: engine.Measurement{
				Last:       100,
				Source:     "pumpdump",
				Type:       engine.Pump,
				Regime:     "microstructure",
				Reason:     "actual_pump",
				Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
				Confidence: 0.8,
			},
			predictedReturn: 0.008,
			anchorPrice:     1.0,
			direction:       1,
			runway:          config.System.ScalpHoldBeforeExit,
			dueAt:           time.Now().Add(-time.Millisecond),
			predictedAt:     time.Now().Add(-config.System.ScalpHoldBeforeExit),
		},
	}

	if err := prediction.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	select {
	case value := <-subscriber.Incoming:
		payload, ok := value.Value.(engine.PredictionFeedback)

		if !ok {
			t.Fatalf("expected prediction feedback, got %T", value.Value)
		}

		if payload.Source != "pumpdump" || payload.Symbol != "PUMP/EUR" {
			t.Fatalf("unexpected feedback: %+v", payload)
		}

		if payload.PredictedReturn <= 0 || payload.ActualReturn <= 0 {
			t.Fatalf("expected positive returns, got predicted=%v actual=%v", payload.PredictedReturn, payload.ActualReturn)
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

func TestPredictionRecord(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	prediction.SeedReturnCalibration("pumpdump", 0.01)

	measurement := engine.Measurement{
		Source:     "pumpdump",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.8,
	}

	predicted := prediction.Record(
		engine.Perspective{Type: engine.PerspectiveMicrostructure},
		measurement,
		50000,
		time.Now(),
	)

	if predicted <= 0 {
		t.Fatalf("expected calibrated predicted return, got %v", predicted)
	}

	if prediction.open["BTC/EUR"]["pumpdump"].predictedReturn != predicted {
		t.Fatalf("expected open prediction stored")
	}
}

func TestPredictionConcurrentRecordAndTick(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 4, 8, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	prediction := NewPrediction(ctx, pool)
	prediction.SeedReturnCalibration("pumpdump", 0.01)
	prediction.prices["BTC/EUR"] = 50000
	prediction.open["BTC/EUR"] = map[string]openPrediction{
		"pumpdump": {
			measurement: engine.Measurement{
				Last:       100,
				Source:     "pumpdump",
				Type:       engine.Pump,
				Regime:     "microstructure",
				Reason:     "actual_pump",
				Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
				Confidence: 0.8,
			},
			predictedReturn: 0.008,
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
	}

	var workers sync.WaitGroup

	for index := 0; index < 32; index++ {
		workers.Add(1)

		go func() {
			defer workers.Done()
			_ = prediction.Tick()
		}()

		workers.Add(1)

		go func() {
			defer workers.Done()
			prediction.Record(
				engine.Perspective{Type: engine.PerspectiveMicrostructure},
				measurement,
				50000,
				time.Now(),
			)
		}()
	}

	workers.Wait()
}
