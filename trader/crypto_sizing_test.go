package trader

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestBatchPeakConfidenceUsesMaxNotFirstRanked(t *testing.T) {
	candidates := []tradeCandidate{
		{symbol: "PUMP/EUR", confidence: 0.3, measType: engine.Pump},
		{symbol: "FLOW/EUR", confidence: 500, measType: engine.Flow},
	}

	peak := batchPeakConfidence(candidates)

	if peak != 500 {
		t.Fatalf("expected peak confidence 500, got %v", peak)
	}
}

func TestEntryNotionalCapsWeightWhenPumpRanksFirst(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, err := NewCrypto(context.Background(), nil, wallet, stubPrices{"FLOW/EUR": 1}, NoopPublisher(), &stubSignal{})

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	notional := crypto.entryNotional(500, 0.3)
	slotCap := 200 * config.System.MaxSlotPct / 100

	if notional != slotCap {
		t.Fatalf("expected notional capped to slot %.4f, got %.4f", slotCap, notional)
	}

	if notional > wallet.Balance {
		t.Fatalf("notional %.4f exceeds wallet balance %.4f", notional, wallet.Balance)
	}
}

func TestTryEnterCapsNotionalWhenPeakUsesLowPriorityHead(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, err := NewCrypto(context.Background(), nil, wallet, stubPrices{"FLOW/EUR": 1}, NoopPublisher(), &stubSignal{})

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.tryEnter(tradeCandidate{
		pair:       asset.Pair{Wsname: "FLOW/EUR"},
		symbol:     "FLOW/EUR",
		confidence: 500,
		regime:     "flow",
		measType:   engine.Flow,
	}, 0.3)

	hold, ok := crypto.holds["FLOW/EUR"]

	if !ok {
		t.Fatal("expected flow entry")
	}

	slotCap := 200 * config.System.MaxSlotPct / 100

	if hold.notional > slotCap+0.01 {
		t.Fatalf("expected notional capped to %.4f, got %.4f", slotCap, hold.notional)
	}

	if wallet.Balance < 0 {
		t.Fatalf("wallet overdrafted to %.4f", wallet.Balance)
	}
}

func TestTradingSolventStopsEntriesWhenEquityGone(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", -100, 0.26)
	stub := &stubSignal{
		measurements: []engine.Measurement{{
			Type: engine.Pump, Regime: "pump",
			Pairs: []asset.Pair{{Wsname: "PUMP/EUR"}}, Confidence: 0.8,
		}},
	}
	crypto, err := NewCrypto(context.Background(), nil, wallet, stubPrices{"PUMP/EUR": 1}, NoopPublisher(), stub)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	if crypto.tradingSolvent() {
		t.Fatal("expected insolvent trader")
	}

	decideForEntry(crypto, stub.measurements)

	if len(crypto.holds) != 0 {
		t.Fatalf("expected no entries while insolvent, got %d", len(crypto.holds))
	}
}

func BenchmarkEntryWeight(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if entryWeight(500, 0.3) != 1 {
			b.Fatal("expected capped weight")
		}
	}
}
