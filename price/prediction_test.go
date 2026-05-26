package price

import (
	"context"
	"math"
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
