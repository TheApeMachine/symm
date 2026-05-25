package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestCryptoBuffersMeasurement(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, wallet)

	if crypto == nil {
		t.Fatal("expected crypto trader")
	}

	crypto.broadcasts["measurements"].Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Regime:     "microstructure",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
			Confidence: 0.8,
		},
	})

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(crypto.measurements) != 1 {
		t.Fatalf("expected one buffered measurement, got %d", len(crypto.measurements))
	}
}

func TestCryptoSettlesFeedback(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	crypto := NewCrypto(ctx, pool, NewWallet(PaperWallet, "EUR", 200, 0.26))
	feedback := pool.CreateBroadcastGroup("feedback", 10*time.Millisecond)
	subscriber := feedback.Subscribe("test:feedback", 8)

	crypto.returnCount["pumpdump"] = config.System.MinCalibrationSamples
	returnEMA := adaptive.NewEMA(0)
	_, _ = returnEMA.Next(0, 0.01)
	crypto.returns["pumpdump"] = returnEMA

	crypto.pairs["PUMP/EUR"] = &pairState{
		lastPrice: 1.02,
		open: map[string]openPrediction{
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
		},
	}

	if err := crypto.Tick(); err != nil {
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

func TestCryptoApplyFill(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, wallet)

	if err := crypto.wallet.ReserveEntry(50); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	crypto.applyFill(order.Fill{
		OrderID: "ORDER-1",
		Symbol:  "PUMP/EUR",
		Side:    "buy",
		Qty:     10,
		Price:   1,
	})

	if crypto.wallet.Inventory["PUMP"] != 10 {
		t.Fatalf("expected inventory 10, got %v", crypto.wallet.Inventory["PUMP"])
	}
}

func TestCryptoCloseCancelsContext(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	crypto := NewCrypto(ctx, pool, NewWallet(PaperWallet, "EUR", 200, 0.26))

	if err := crypto.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func BenchmarkCryptoTick(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	crypto := NewCrypto(ctx, pool, NewWallet(PaperWallet, "EUR", 200, 0.26))
	crypto.pairs["PUMP/EUR"] = &pairState{
		lastPrice: 1.0,
		open:      make(map[string]openPrediction),
	}

	measurement := engine.Measurement{
		Source:     "pumpdump",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
		Confidence: 0.8,
	}

	b.ReportAllocs()

	for b.Loop() {
		crypto.measurements = append(crypto.measurements[:0], measurement)
		_ = crypto.Tick()
	}
}
