package trader

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
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
