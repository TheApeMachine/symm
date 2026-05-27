package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
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
			Last:       1.0,
			Bid:        0.99,
			Ask:        1.01,
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
			Last:       1.0,
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
				Last:       1.0,
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
				Last:       1.0,
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

func TestCryptoEnterPaperRequiresFusion(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, predictions, pool := newTestCrypto(t, wallet)

	crypto.pulses = config.System.MinWarmPulses
	predictions.SeedReturnCalibration("pumpdump", 0.01)

	pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Regime:     "microstructure",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.8,
			Last:       50000,
			Bid:        49999,
			Ask:        50001,
		},
	})

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if wallet.Inventory["BTC"] > config.System.LiveInventoryEpsilon {
		t.Fatalf("expected fused gate to block single-source entry, got %v", wallet.Inventory["BTC"])
	}
}

func TestCryptoDefendsRestingEntry(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, _, pool := newTestCrypto(t, wallet)

	crypto.restingEntries["BTC/EUR"] = restingEntry{
		Symbol:     "BTC/EUR",
		LimitPrice: 50000,
		Notional:   10,
		PlacedAt:   time.Now(),
	}

	if err := crypto.wallet.ReserveEntry(10); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "depthflow",
			Type:       engine.Dump,
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.9,
			Last:       50000,
		},
	})

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if _, ok := crypto.restingEntries["BTC/EUR"]; ok {
		t.Fatal("expected resting entry to be canceled")
	}

	if wallet.ReservedEUR > 0 {
		t.Fatalf("expected reservation released, reserved=%v", wallet.ReservedEUR)
	}
}

func TestCryptoEnterPaper(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, predictions, pool := newTestCrypto(t, wallet)

	crypto.pulses = config.System.MinWarmPulses
	predictions.SeedReturnCalibration("pumpdump", 0.03)
	predictions.SeedReturnCalibration("hawkes", 0.03)

	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	for _, measurement := range []engine.Measurement{
		{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Regime:     "microstructure",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.8,
			Last:       50000,
			Bid:        49999,
			Ask:        50001,
		},
		{
			Source:     "hawkes",
			Type:       engine.Momentum,
			Regime:     "microstructure",
			Reason:     "cluster_buy",
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.7,
			Last:       50000,
			Bid:        49999,
			Ask:        50001,
		},
	} {
		measurements.Send(&qpool.QValue[any]{Value: measurement})
	}

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if wallet.Inventory["BTC"] <= 0 {
		t.Fatalf("expected paper entry inventory, got %v", wallet.Inventory["BTC"])
	}
}

func TestCryptoCloseCancelsContext(t *testing.T) {
	crypto, _, _ := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

	if err := crypto.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestCryptoHandleExitPaper(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["BTC"] = 0.01

	crypto, predictions, pool := newTestCrypto(t, wallet)

	pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "BTC/EUR", Last: 50000},
	})

	if err := predictions.Tick(); err != nil {
		t.Fatalf("prediction tick: %v", err)
	}

	pool.CreateBroadcastGroup("exits", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Exit{
			Symbol:  "BTC/EUR",
			Urgency: 0.9,
			Reason:  "depth_decay",
		},
	})

	balanceBefore := wallet.Balance

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if wallet.Inventory["BTC"] > config.System.LiveInventoryEpsilon {
		t.Fatalf("expected flat BTC inventory, got %v", wallet.Inventory["BTC"])
	}

	if wallet.Balance <= balanceBefore {
		t.Fatalf("expected balance increase after exit, before=%v after=%v", balanceBefore, wallet.Balance)
	}
}

func TestCryptoHandleExitLive(t *testing.T) {
	wallet := NewWallet(CryptoWallet, "EUR", 200, 0.26)
	wallet.Inventory["BTC"] = 0.01

	crypto, _, pool := newTestCrypto(t, wallet)
	orders := pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Subscribe("test:orders", 8)

	pool.CreateBroadcastGroup("exits", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Exit{
			Symbol:  "BTC/EUR",
			Urgency: 0.9,
			Reason:  "depth_decay",
		},
	})

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	select {
	case value := <-orders.Incoming:
		if value.Value == nil {
			t.Fatal("expected live sell order")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live sell order")
	}
}

func TestCryptoHandleExitNoInventory(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

	pool.CreateBroadcastGroup("exits", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Exit{
			Symbol:  "BTC/EUR",
			Urgency: 0.9,
			Reason:  "depth_decay",
		},
	})

	if err := crypto.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
}

func TestCryptoHandleExitInvalidPayload(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

	pool.CreateBroadcastGroup("exits", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: map[string]any{"symbol": "BTC/EUR"},
	})

	if err := crypto.Tick(); err == nil {
		t.Fatal("expected error for invalid exit payload")
	}
}

func BenchmarkCryptoTick(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	predictions := price.NewPrediction(ctx, pool)
	crypto := NewCrypto(ctx, pool, NewWallet(PaperWallet, "EUR", 200, 0.26), predictions)
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	measurement := engine.Measurement{
		Source:     "pumpdump",
		Type:       engine.Pump,
		Regime:     "microstructure",
		Reason:     "actual_pump",
		Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
		Confidence: 0.8,
		Last:       1.0,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		measurements.Send(&qpool.QValue[any]{Value: measurement})
		_ = crypto.Tick()
	}
}
