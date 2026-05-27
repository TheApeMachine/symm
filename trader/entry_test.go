package trader

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/price"
)

func TestMakerMaxChasePrice(t *testing.T) {
	original := config.System.MaxEntrySlippageBPS
	config.System.MaxEntrySlippageBPS = 50
	t.Cleanup(func() { config.System.MaxEntrySlippageBPS = original })

	maxPrice := makerMaxChasePrice(100)

	if math.Abs(maxPrice-100.5) > 1e-9 {
		t.Fatalf("expected max chase 100.5, got %v", maxPrice)
	}
}

func TestChaseRestingEntry(t *testing.T) {
	originalSlippage := config.System.MaxEntrySlippageBPS
	config.System.MaxEntrySlippageBPS = 50
	t.Cleanup(func() { config.System.MaxEntrySlippageBPS = originalSlippage })

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, _, _ := newTestCrypto(t, wallet)

	crypto.restingEntries["BTC/EUR"] = restingEntry{
		Symbol:     "BTC/EUR",
		LimitPrice: 50000,
		AnchorBid:  50000,
		Notional:   10,
		PlacedAt:   time.Now(),
	}

	crypto.chaseRestingEntries([]engine.Measurement{{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Bid:   50010,
		Ask:   50012,
		Last:  50010,
	}})

	entry, ok := crypto.restingEntries["BTC/EUR"]

	if !ok {
		t.Fatal("expected resting entry to remain after chase")
	}

	if entry.LimitPrice != 50010 {
		t.Fatalf("expected chased limit 50010, got %v", entry.LimitPrice)
	}
}

func TestAbandonRestingEntryOnChaseSlippage(t *testing.T) {
	originalSlippage := config.System.MaxEntrySlippageBPS
	config.System.MaxEntrySlippageBPS = 50
	t.Cleanup(func() { config.System.MaxEntrySlippageBPS = originalSlippage })

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, _, _ := newTestCrypto(t, wallet)

	crypto.restingEntries["BTC/EUR"] = restingEntry{
		Symbol:     "BTC/EUR",
		LimitPrice: 50000,
		AnchorBid:  50000,
		Notional:   10,
		PlacedAt:   time.Now(),
	}

	if err := crypto.wallet.ReserveEntry(10); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	crypto.chaseRestingEntries([]engine.Measurement{{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Bid:   50300,
		Ask:   50302,
		Last:  50300,
	}})

	if _, ok := crypto.restingEntries["BTC/EUR"]; ok {
		t.Fatal("expected resting entry abandoned beyond chase slippage")
	}

	if wallet.ReservedEUR > 0 {
		t.Fatalf("expected reservation released, reserved=%v", wallet.ReservedEUR)
	}
}

func TestEnterBlocksExistingInventory(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["BTC"] = 0.01
	crypto, _, _ := newTestCrypto(t, wallet)

	crypto.enter(tradeOpportunity{
		Measurement: engine.Measurement{
			Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
			Last:  50000,
			Bid:   49999,
			Ask:   50001,
		},
	}, 10)

	if _, ok := crypto.restingEntries["BTC/EUR"]; ok {
		t.Fatal("expected no resting entry when inventory already open")
	}

	if wallet.ReservedEUR > 0 {
		t.Fatalf("expected no reservation, reserved=%v", wallet.ReservedEUR)
	}
}

func TestScoreDoesNotReenterOpenSymbol(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["BTC"] = 0.01
	crypto, predictions, _ := newTestCrypto(t, wallet)

	crypto.pulses = config.System.MinWarmPulses
	predictions.SeedReturnCalibration("pumpdump", 0.03)
	predictions.SeedReturnCalibration("hawkes", 0.03)

	beforeQty := wallet.Inventory["BTC"]

	if err := crypto.score([]engine.Measurement{
		{
			Source:     "pumpdump",
			Type:       engine.Pump,
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.8,
			Last:       50000,
			Bid:        49999,
			Ask:        50001,
		},
		{
			Source:     "hawkes",
			Type:       engine.Momentum,
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.7,
			Last:       50000,
			Bid:        49999,
			Ask:        50001,
		},
	}); err != nil {
		t.Fatalf("score: %v", err)
	}

	if wallet.Inventory["BTC"] != beforeQty {
		t.Fatalf("expected unchanged BTC inventory, got %v", wallet.Inventory["BTC"])
	}
}

func TestPaperMakerEntryFill(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, _, _ := newTestCrypto(t, wallet)

	crypto.enterMaker("BTC/EUR", 50000, 50001, 10)

	crypto.fillRestingEntries([]engine.Measurement{{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Bid:   50000,
		Ask:   50001,
		Last:  50000,
	}})

	if wallet.Inventory["BTC"] <= 0 {
		t.Fatalf("expected paper maker fill inventory, got %v", wallet.Inventory["BTC"])
	}
}

func TestSettlePaperEntryChargesMakerFeeOnce(t *testing.T) {
	originalFee := config.System.MakerFeePct
	config.System.MakerFeePct = 1
	t.Cleanup(func() { config.System.MakerFeePct = originalFee })

	wallet := NewWallet(PaperWallet, "EUR", 100, 1)
	crypto, _, _ := newTestCrypto(t, wallet)

	crypto.enterMaker("BTC/EUR", 10, 10.01, 100)
	crypto.fillRestingEntries([]engine.Measurement{{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Bid:   10,
		Ask:   10.01,
		Last:  10,
	}})

	if math.Abs(wallet.Balance) > 1e-9 {
		t.Fatalf("expected full quote budget spent, balance=%v", wallet.Balance)
	}

	if math.Abs(wallet.Inventory["BTC"]-9.9) > 1e-9 {
		t.Fatalf("expected maker fee deducted once from base, inventory=%v", wallet.Inventory["BTC"])
	}
}

func TestEnterTakerChargesFeeOnce(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 100, 1)
	crypto, _, _ := newTestCrypto(t, wallet)

	crypto.enterTaker("BTC/EUR", 10, 10, 10, 100, engine.Measurement{})

	if math.Abs(wallet.Balance) > 1e-9 {
		t.Fatalf("expected full quote budget spent, balance=%v", wallet.Balance)
	}

	if math.Abs(wallet.Inventory["BTC"]-9.9) > 1e-9 {
		t.Fatalf("expected taker fee deducted once from base, inventory=%v", wallet.Inventory["BTC"])
	}
}

func BenchmarkEnterTakerPaper(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	wallet := NewWallet(PaperWallet, "EUR", 1e12, 0.26)
	predictions := price.NewPrediction(ctx, pool)
	crypto := NewCrypto(ctx, pool, wallet, predictions)
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Last:  10,
		Bid:   10,
		Ask:   10,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		crypto.enterTaker("BTC/EUR", 10, 10, 10, 10, measurement)
	}
}
