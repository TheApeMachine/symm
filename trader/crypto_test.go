package trader

import (
	"context"
	"errors"
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

func startCryptoTick(t *testing.T, crypto *Crypto) {
	t.Helper()

	go func() {
		if err := crypto.Tick(); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("crypto tick: %v", err)
		}
	}()

	t.Cleanup(func() {
		crypto.cancel()
		time.Sleep(10 * time.Millisecond)
	})
}

func waitForCryptoPulse(t *testing.T, crypto *Crypto, want int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		if crypto.pulses >= want {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("expected %d score pulses, got %d", want, crypto.pulses)
}

func TestCryptoScoresMeasurement(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

	startCryptoTick(t, crypto)

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

	waitForCryptoPulse(t, crypto, 1)
}

func TestCryptoScoreBatch(t *testing.T) {
	crypto, _, _ := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

	if err := crypto.scoreBatch(engine.Measurement{
		Source:     "hawkes",
		Type:       engine.Momentum,
		Regime:     "microstructure",
		Reason:     "buy_cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.55,
		Last:       50000,
	}); err != nil {
		t.Fatalf("score batch: %v", err)
	}

	if crypto.pulses != 1 {
		t.Fatalf("expected one score pulse, got %d", crypto.pulses)
	}
}

func TestCryptoPublishConfidence(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))
	confidenceSub := crypto.broadcasts["confidence"].Subscribe("test:confidence", 8)

	startCryptoTick(t, crypto)

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

func TestCryptoPublishConfidenceEMA(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))
	confidenceSub := crypto.broadcasts["confidence"].Subscribe("test:confidence", 8)
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	startCryptoTick(t, crypto)

	measurements.Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "fluid",
			Type:       engine.Flow,
			Regime:     "fluid",
			Reason:     "field_activity",
			Pairs:      []asset.Pair{{Wsname: "ALT/EUR"}},
			Confidence: 0.4,
			Last:       1.0,
			Bid:        0.99,
			Ask:        1.01,
		},
	})

	<-confidenceSub.Incoming

	measurements.Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "fluid",
			Type:       engine.Flow,
			Regime:     "fluid",
			Reason:     "field_activity",
			Pairs:      []asset.Pair{{Wsname: "ALT/EUR"}},
			Confidence: 0.6,
			Last:       1.0,
			Bid:        0.99,
			Ask:        1.01,
		},
	})

	select {
	case value := <-confidenceSub.Incoming:
		payload, ok := value.Value.(map[string]any)

		if !ok {
			t.Fatalf("expected map payload, got %T", value.Value)
		}

		confidence, ok := payload["confidence"].(float64)

		if !ok || confidence != crypto.sourceConfidence["fluid"].Value() {
			t.Fatalf("expected fluid EMA %v, got %v", crypto.sourceConfidence["fluid"].Value(), payload["confidence"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for confidence EMA publish")
	}
}

func TestCryptoAttachWalletMarksPrefersLastPrice(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["A"] = 10
	wallet.AvgEntry = map[string]float64{"A": 0.07}

	crypto, predictions, pool := newTestCrypto(t, wallet)

	go func() {
		_ = predictions.Tick()
	}()
	t.Cleanup(func() {
		_ = predictions.Close()
		time.Sleep(10 * time.Millisecond)
	})

	pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "A/EUR", Last: 0.081},
	})

	time.Sleep(20 * time.Millisecond)

	crypto.attachWalletMarks()

	if wallet.Marks["A/EUR"] != 0.081 {
		t.Fatalf("expected last ticker price 0.081, got %v", wallet.Marks["A/EUR"])
	}
}

func TestCryptoObserveTickerUpdatesMarks(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["MNT"] = 10
	wallet.AvgEntry = map[string]float64{"MNT": 0.55}

	crypto, _, pool := newTestCrypto(t, wallet)
	walletSub := crypto.broadcasts["wallet"].Subscribe("test:wallet", 8)

	startCryptoTick(t, crypto)

	pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: market.TickerRow{
			Symbol: "MNT/EUR",
			Last:   0.57,
		},
	})

	select {
	case value := <-walletSub.Incoming:
		payload, ok := value.Value.(*Wallet)

		if !ok {
			t.Fatalf("expected wallet payload, got %T", value.Value)
		}

		mark := payload.Marks["MNT/EUR"]

		if mark != 0.57 {
			t.Fatalf("expected mark 0.57, got %v", mark)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for wallet mark update")
	}
}

func TestCryptoSendWalletPublishesSnapshot(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["BTC"] = 0.01
	wallet.AvgEntry["BTC"] = 50000

	crypto, _, _ := newTestCrypto(t, wallet)
	crypto.portfolioRisk.lastPrices["BTC/EUR"] = 50100
	walletSub := crypto.broadcasts["wallet"].Subscribe("test:wallet-snapshot", 8)

	crypto.sendWallet()

	select {
	case value := <-walletSub.Incoming:
		payload, ok := value.Value.(*Wallet)

		if !ok {
			t.Fatalf("expected wallet payload, got %T", value.Value)
		}

		if payload == wallet {
			t.Fatal("expected wallet snapshot, got live wallet pointer")
		}

		wallet.Inventory["BTC"] = 0.02
		wallet.AvgEntry["BTC"] = 51000
		wallet.Marks["BTC/EUR"] = 50200

		if payload.Inventory["BTC"] != 0.01 {
			t.Fatalf("expected immutable inventory snapshot, got %v", payload.Inventory["BTC"])
		}

		if payload.AvgEntry["BTC"] != 50000 {
			t.Fatalf("expected immutable average entry snapshot, got %v", payload.AvgEntry["BTC"])
		}

		if payload.Marks["BTC/EUR"] != 50100 {
			t.Fatalf("expected immutable mark snapshot, got %v", payload.Marks["BTC/EUR"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for wallet snapshot")
	}
}

func TestCryptoPublishConfidenceMean(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))
	confidenceSub := crypto.broadcasts["confidence"].Subscribe("test:confidence", 8)
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	startCryptoTick(t, crypto)

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

	waitForCryptoPulse(t, crypto, 1)

	expected := crypto.sourceConfidence["pumpdump"].Value()

	select {
	case value := <-confidenceSub.Incoming:
		payload, ok := value.Value.(map[string]any)

		if !ok {
			t.Fatalf("expected map payload, got %T", value.Value)
		}

		confidence, ok := payload["confidence"].(float64)

		if !ok || confidence != expected {
			t.Fatalf("expected EMA confidence %v, got %v", expected, payload["confidence"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for confidence publish")
	}
}

func TestCryptoPublishConfidenceBatchFromChannel(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))
	confidenceSub := crypto.broadcasts["confidence"].Subscribe("test:confidence", 8)
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	startCryptoTick(t, crypto)

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

	waitForCryptoPulse(t, crypto, 1)

	select {
	case value := <-confidenceSub.Incoming:
		payload, ok := value.Value.(map[string]any)

		if !ok {
			t.Fatalf("expected map payload, got %T", value.Value)
		}

		confidence, ok := payload["confidence"].(float64)

		if !ok || confidence != crypto.sourceConfidence["pumpdump"].Value() {
			t.Fatalf("expected published EMA %v, got %v", crypto.sourceConfidence["pumpdump"].Value(), payload["confidence"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for confidence publish")
	}
}

func TestCryptoPublishConfidenceRepublishesAllSources(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))
	confidenceSub := crypto.broadcasts["confidence"].Subscribe("test:confidence", 16)
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	startCryptoTick(t, crypto)

	measurements.Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Regime:     "microstructure",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "A/EUR"}},
			Confidence: 0.6,
			Last:       1.0,
		},
	})

	<-confidenceSub.Incoming

	measurements.Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "hawkes",
			Type:       engine.Momentum,
			Regime:     "microstructure",
			Reason:     "cluster_buy",
			Pairs:      []asset.Pair{{Wsname: "B/EUR"}},
			Confidence: 0.7,
			Last:       1.0,
		},
	})

	published := make(map[string]float64)

	for range 2 {
		select {
		case value := <-confidenceSub.Incoming:
			payload, ok := value.Value.(map[string]any)

			if !ok {
				t.Fatalf("expected map payload, got %T", value.Value)
			}

			source, _ := payload["source"].(string)
			confidence, _ := payload["confidence"].(float64)
			published[source] = confidence
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for confidence republish")
		}
	}

	if published["pumpdump"] != 0.6 {
		t.Fatalf("expected pumpdump EMA 0.6 republished, got %v", published["pumpdump"])
	}

	if published["hawkes"] != 0.7 {
		t.Fatalf("expected hawkes EMA 0.7, got %v", published["hawkes"])
	}
}

func TestCryptoEnterPaperRequiresFusion(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, predictions, pool := newTestCrypto(t, wallet)

	crypto.pulses = config.System.MinWarmPulses
	predictions.SeedReturnCalibration("pumpdump", 0.01)

	startCryptoTick(t, crypto)

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

	waitForCryptoPulse(t, crypto, 1)

	if wallet.Inventory["BTC"] > config.System.LiveInventoryEpsilon {
		t.Fatalf("expected fused gate to block single-source entry, got %v", wallet.Inventory["BTC"])
	}
}

func TestCryptoDoesNotEnterBeforePredictionCalibration(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, _, _ := newTestCrypto(t, wallet)

	crypto.pulses = config.System.MinWarmPulses

	if err := crypto.score([]engine.Measurement{
		{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Regime:     "microstructure",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.95,
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
			Confidence: 0.9,
			Last:       50000,
			Bid:        49999,
			Ask:        50001,
		},
	}); err != nil {
		t.Fatalf("score: %v", err)
	}

	if wallet.Inventory["BTC"] > config.System.LiveInventoryEpsilon {
		t.Fatalf("expected uncalibrated forecasts to block entry, got %v", wallet.Inventory["BTC"])
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

	startCryptoTick(t, crypto)

	pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Measurement{
			Source:     "depthflow",
			Type:       engine.Dump,
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.9,
			Last:       50000,
		},
	})

	waitForCryptoPulse(t, crypto, 1)

	if _, ok := crypto.restingEntries["BTC/EUR"]; ok {
		t.Fatal("expected resting entry to be canceled")
	}

	if wallet.ReservedEUR > 0 {
		t.Fatalf("expected reservation released, reserved=%v", wallet.ReservedEUR)
	}
}

func TestScorePaperMakerEntry(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, predictions, _ := newTestCrypto(t, wallet)

	crypto.pulses = config.System.MinWarmPulses
	predictions.SeedReturnCalibration("pumpdump", 0.03)
	predictions.SeedReturnCalibration("hawkes", 0.03)

	batch := []engine.Measurement{
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
	}

	if err := crypto.score(batch); err != nil {
		t.Fatalf("score: %v", err)
	}

	if wallet.Inventory["BTC"] <= 0 {
		t.Fatalf("expected paper entry inventory, got %v resting=%v", wallet.Inventory["BTC"], crypto.restingEntries)
	}
}

func TestCryptoEnterPaper(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, predictions, pool := newTestCrypto(t, wallet)

	crypto.pulses = config.System.MinWarmPulses
	predictions.SeedReturnCalibration("pumpdump", 0.03)
	predictions.SeedReturnCalibration("hawkes", 0.03)

	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	startCryptoTick(t, crypto)

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

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		if wallet.Inventory["BTC"] > config.System.LiveInventoryEpsilon {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("expected paper entry inventory, got %v resting=%v", wallet.Inventory["BTC"], crypto.restingEntries)
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

	go func() {
		_ = predictions.Tick()
	}()
	t.Cleanup(func() {
		_ = predictions.Close()
		time.Sleep(10 * time.Millisecond)
	})

	startCryptoTick(t, crypto)

	pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "BTC/EUR", Last: 50000},
	})

	time.Sleep(20 * time.Millisecond)

	pool.CreateBroadcastGroup("exits", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Exit{
			Symbol:  "BTC/EUR",
			Urgency: 0.9,
			Reason:  "depth_decay",
		},
	})

	balanceBefore := wallet.Balance

	time.Sleep(50 * time.Millisecond)

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

	startCryptoTick(t, crypto)

	pool.CreateBroadcastGroup("exits", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Exit{
			Symbol:  "BTC/EUR",
			Urgency: 0.9,
			Reason:  "depth_decay",
		},
	})

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

	startCryptoTick(t, crypto)

	pool.CreateBroadcastGroup("exits", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: engine.Exit{
			Symbol:  "BTC/EUR",
			Urgency: 0.9,
			Reason:  "depth_decay",
		},
	})

	time.Sleep(50 * time.Millisecond)
}

func TestCryptoHandleExitInvalidPayload(t *testing.T) {
	crypto, _, pool := newTestCrypto(t, NewWallet(PaperWallet, "EUR", 200, 0.26))

	startCryptoTick(t, crypto)

	pool.CreateBroadcastGroup("exits", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: map[string]any{"symbol": "BTC/EUR"},
	})

	time.Sleep(50 * time.Millisecond)
}

func BenchmarkCryptoScoreBatch(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	predictions := price.NewPrediction(ctx, pool)
	crypto := NewCrypto(ctx, pool, NewWallet(PaperWallet, "EUR", 200, 0.26), predictions)

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
		_ = crypto.scoreBatch(measurement)
	}
}
