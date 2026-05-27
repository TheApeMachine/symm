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
	"github.com/theapemachine/symm/price"
)

func newTestCrypto(t *testing.T, wallet *Wallet) (*Crypto, *price.Prediction, *qpool.Q) {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	predictions := price.NewPrediction(ctx, pool)
	crypto := NewCrypto(ctx, pool, wallet, predictions)

	if crypto == nil {
		t.Fatal("expected crypto trader")
	}

	return crypto, predictions, pool
}

func TestCryptoScoresMeasurement(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

	pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).Send(&qpool.QValue[any]{
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

	if crypto.pulses != 1 {
		t.Fatalf("expected one score pulse, got %d", crypto.pulses)
	}
}

func TestCryptoPublishConfidence(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))
	confidenceSub := crypto.broadcasts["confidence"].Subscribe("test:confidence", 8)

	pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Regime:     "microstructure",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
			Confidence: 0.6,
		},
	})

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	select {
	case value := <-confidenceSub.Incoming:
		payload, ok := value.Value.(map[string]any)

		if !ok {
			t.Fatalf("expected map payload, got %T", value.Value)
		}

		if payload["source"] != "pumpdump" {
			t.Fatalf("expected pumpdump source, got %v", payload["source"])
		}

		confidence, ok := payload["confidence"].(float64)

		if !ok || confidence != 0.6 {
			t.Fatalf("expected confidence 0.6, got %v", payload["confidence"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for confidence publish")
	}
}

func TestCryptoPublishConfidenceMean(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))
	confidenceSub := crypto.broadcasts["confidence"].Subscribe("test:confidence", 8)
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	for _, pair := range []struct {
		symbol     string
		confidence float64
	}{
		{"A/EUR", 0.2},
		{"B/EUR", 0.8},
	} {
		measurements.Send(&qpool.QValue[any]{
			Value: engine.Measurement{
				Source:     "pumpdump",
				Type:       engine.Pump,
				Regime:     "microstructure",
				Reason:     "actual_pump",
				Pairs:      []asset.Pair{{Wsname: pair.symbol}},
				Confidence: pair.confidence,
			},
		})
	}

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	select {
	case value := <-confidenceSub.Incoming:
		payload, ok := value.Value.(map[string]any)

		if !ok {
			t.Fatalf("expected map payload, got %T", value.Value)
		}

		confidence, ok := payload["confidence"].(float64)

		if !ok || confidence != 0.5 {
			t.Fatalf("expected mean confidence 0.5, got %v", payload["confidence"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for confidence publish")
	}
}

func TestCryptoPublishConfidenceBatchFromChannel(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))
	confidenceSub := crypto.broadcasts["confidence"].Subscribe("test:confidence", 8)
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	for _, pair := range []struct {
		symbol     string
		confidence float64
	}{
		{"A/EUR", 0.2},
		{"B/EUR", 0.8},
		{"C/EUR", 0.5},
	} {
		measurements.Send(&qpool.QValue[any]{
			Value: engine.Measurement{
				Source:     "pumpdump",
				Type:       engine.Pump,
				Regime:     "microstructure",
				Reason:     "actual_pump",
				Pairs:      []asset.Pair{{Wsname: pair.symbol}},
				Confidence: pair.confidence,
			},
		})
	}

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	select {
	case value := <-confidenceSub.Incoming:
		payload, ok := value.Value.(map[string]any)

		if !ok {
			t.Fatalf("expected map payload, got %T", value.Value)
		}

		confidence, ok := payload["confidence"].(float64)

		if !ok || confidence != 0.5 {
			t.Fatalf("expected mean confidence 0.5, got %v", payload["confidence"])
		}

		count, ok := payload["count"].(int)

		if !ok || count != 3 {
			t.Fatalf("expected count 3, got %v", payload["count"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for confidence publish")
	}
}

func TestCryptoEnterPaper(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, predictions, pool := newTestCrypto(t, wallet)

	crypto.pulses = config.System.MinWarmPulses
	predictions.SeedReturnCalibration("pumpdump", 0.01)
	crypto.pairs["BTC/EUR"] = &pairState{
		lastPrice: 50000,
		bid:       49999,
		ask:       50001,
	}

	pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Regime:     "microstructure",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.8,
		},
	})

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if wallet.Inventory["BTC"] <= 0 {
		t.Fatalf("expected paper entry inventory, got %v", wallet.Inventory["BTC"])
	}
}

func TestCryptoApplyFill(t *testing.T) {
	crypto, _, _ := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

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
	crypto, _, _ := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

	if err := crypto.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func BenchmarkCryptoTick(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	predictions := price.NewPrediction(ctx, pool)
	crypto := NewCrypto(ctx, pool, NewWallet(PaperWallet, "EUR", 200, 0.26), predictions)
	crypto.pairs["PUMP/EUR"] = &pairState{lastPrice: 1.0}
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	measurement := engine.Measurement{
		Source:     "pumpdump",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
		Confidence: 0.8,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		measurements.Send(&qpool.QValue[any]{Value: measurement})
		_ = crypto.Tick()
	}
}
