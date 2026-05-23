package trader

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

type stubPrices map[string]float64

func (stub stubPrices) Last(symbol string) (float64, bool) {
	price, ok := stub[symbol]
	return price, ok
}

func (stub stubPrices) Quote(symbol string) (last, bid, ask, changePct float64, ok bool) {
	last, ok = stub[symbol]
	if !ok {
		return 0, 0, 0, 0, false
	}

	return last, last * 0.999, last * 1.001, 0, true
}

func (stub stubPrices) Timestamp(_ string) (string, bool) {
	return time.Now().UTC().Format(time.RFC3339Nano), true
}

type stubSignal struct {
	measurements []engine.Measurement
}

type stubEngineStats struct {
	tickerReady func() int
	symbolTotal func() int
}

func (stats stubEngineStats) TickerReadyCount() int  { return stats.tickerReady() }
func (stats stubEngineStats) SymbolTotal() int       { return stats.symbolTotal() }
func (stats stubEngineStats) FluidSampledCount() int { return 0 }
func (stats stubEngineStats) FluidWarmingCount() int { return 0 }

func primeDecideCrypto(crypto *Crypto) {
	crypto.engineStats = stubEngineStats{
		tickerReady: func() int { return 100 },
		symbolTotal: func() int { return 100 },
	}
	crypto.pulseSeq.Store(int64(config.System.MinWarmPulses))
}

func decideForEntry(crypto *Crypto, batch []engine.Measurement) {
	primeDecideCrypto(crypto)
	crypto.decide(batch)
	crypto.decide(batch)
}

func (stub *stubSignal) Run() {}

func (stub *stubSignal) Measure(_ context.Context) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		for _, measurement := range stub.measurements {
			if !yield(measurement) {
				return
			}
		}
	}
}

func TestCryptoAppliesPumpMeasurement(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	stub := &stubSignal{
		measurements: []engine.Measurement{{
			Type:       engine.Pump,
			Regime:     "pump",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{{Wsname: "PUMP/EUR", Base: "PUMP", Quote: "EUR"}},
			Confidence: 0.8,
		}},
	}
	prices := stubPrices{"PUMP/EUR": 1.0}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), stub)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	startBalance := wallet.Balance

	crypto.tryEnter(tradeCandidate{
		pair:       stub.measurements[0].Pairs[0],
		symbol:     "PUMP/EUR",
		confidence: stub.measurements[0].Confidence,
		support:    1,
		regime:     "pump",
		reason:     "actual_pump",
		measType:   engine.Pump,
	}, stub.measurements[0].Confidence)

	if wallet.Balance >= startBalance {
		t.Fatalf("expected wallet debited, start=%v now=%v", startBalance, wallet.Balance)
	}

	hold := crypto.holds["PUMP/EUR"]
	expectedNotional := 200 * config.System.MaxSlotPct / 100

	if hold.notional != expectedNotional {
		t.Fatalf("expected notional %.4f, got %.4f", expectedNotional, hold.notional)
	}

	if len(crypto.holds) != 1 {
		t.Fatalf("expected one held position, got %d", len(crypto.holds))
	}
}

func TestCryptoAggregatesMultipleSignals(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	pair := asset.Pair{Wsname: "PUMP/EUR", Base: "PUMP", Quote: "EUR"}
	first := &stubSignal{measurements: []engine.Measurement{{
		Type: engine.Pump, Regime: "pump", Pairs: []asset.Pair{pair}, Confidence: 0.4,
	}}}
	second := &stubSignal{measurements: []engine.Measurement{{
		Type: engine.Momentum, Regime: "momentum", Pairs: []asset.Pair{pair}, Confidence: 0.5,
	}}}
	prices := stubPrices{"PUMP/EUR": 1.0}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), first, second)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	decideForEntry(crypto, append(first.measurements, second.measurements...))

	hold, ok := crypto.holds["PUMP/EUR"]

	if !ok {
		t.Fatal("expected aggregated entry")
	}

	if hold.confidence != 0.9 {
		t.Fatalf("expected combined confidence 0.9, got %v", hold.confidence)
	}

	if hold.regime != "pump" {
		t.Fatalf("expected pump regime to win, got %q", hold.regime)
	}
}

func TestCryptoEntersMomentumWithoutPump(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	pair := asset.Pair{Wsname: "SCALP/EUR"}
	stub := &stubSignal{measurements: []engine.Measurement{{
		Type: engine.Momentum, Regime: "momentum", Reason: "cluster_buy",
		Pairs: []asset.Pair{pair}, Confidence: 0.7,
	}}}
	prices := stubPrices{"SCALP/EUR": 2.0}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), stub)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	decideForEntry(crypto, stub.measurements)

	hold, ok := crypto.holds["SCALP/EUR"]

	if !ok {
		t.Fatal("expected momentum entry")
	}

	if hold.regime != "momentum" {
		t.Fatalf("expected momentum regime, got %q", hold.regime)
	}
}

func TestCryptoScalesNotionalByConfidence(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	stub := &stubSignal{}
	prices := stubPrices{"PUMP/EUR": 1.0}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), stub)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	strongNotional := crypto.entryNotional(0.8, 0.8)
	weakNotional := crypto.entryNotional(0.4, 0.8)
	cap := 200 * config.System.MaxSlotPct / 100

	if strongNotional != cap {
		t.Fatalf("expected strong notional %.4f, got %.4f", cap, strongNotional)
	}

	if weakNotional != cap/2 {
		t.Fatalf("expected weak notional %.4f, got %.4f", cap/2, weakNotional)
	}
}

func TestCryptoBoostsRepeatWinner(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	stub := &stubSignal{}
	prices := stubPrices{"PUMP/EUR": 1.0}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), stub)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.records["PUMP/EUR"] = symbolRecord{wins: 2}

	boosted := crypto.boostConfidence("PUMP/EUR", 0.6)
	expected := 0.6 * repeatBoost(2)

	if boosted != expected {
		t.Fatalf("expected boosted confidence %.4f, got %.4f", expected, boosted)
	}
}

func TestCryptoMarkExitsRecordsWinAndFreesSlot(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	stub := &stubSignal{}
	prices := stubPrices{"PUMP/EUR": 1.2}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), stub)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.holds["PUMP/EUR"] = position{
		pair:       asset.Pair{Wsname: "PUMP/EUR"},
		notional:   10,
		entryPrice: 1.0,
		entryFee:   config.System.TakerFee(10, wallet.FeePct),
		enteredAt:  time.Now().Add(-config.System.MinHoldBeforeRotate - time.Second),
		confidence: 0.8,
		regime:     "pump",
		trailPct:   0.01,
		stopPrice:  0.95,
		peakPrice:  1.2,
	}

	if err := crypto.markExits(); err != nil {
		t.Fatalf("mark exits: %v", err)
	}

	if len(crypto.holds) != 0 {
		t.Fatalf("expected position closed, still holding %d", len(crypto.holds))
	}

	if crypto.records["PUMP/EUR"].wins != 1 {
		t.Fatalf("expected one recorded win, got %d", crypto.records["PUMP/EUR"].wins)
	}
}

func TestCryptoReentryUsesWinBoost(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	stub := &stubSignal{
		measurements: []engine.Measurement{{
			Type:       engine.Pump,
			Regime:     "pump",
			Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
			Confidence: 0.8,
		}},
	}
	prices := stubPrices{"PUMP/EUR": 1.0}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), stub)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.records["PUMP/EUR"] = symbolRecord{wins: 1}

	decideForEntry(crypto, stub.measurements)

	hold := crypto.holds["PUMP/EUR"]
	expectedConfidence := 0.8 * repeatBoost(1)

	if hold.confidence != expectedConfidence {
		t.Fatalf("expected boosted confidence %.4f, got %.4f", expectedConfidence, hold.confidence)
	}
}

func TestTryEnterSkipsWithoutPrice(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, err := NewCrypto(context.Background(), nil, wallet, stubPrices{}, NoopPublisher(), &stubSignal{})

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	start := wallet.Balance

	crypto.tryEnter(tradeCandidate{
		symbol:     "MISSING/EUR",
		confidence: 0.8,
		regime:     "momentum",
		measType:   engine.Momentum,
	}, 0.8)

	if wallet.Balance != start {
		t.Fatal("expected skip without price to leave wallet unchanged")
	}
}

func BenchmarkCryptoBoostConfidence(b *testing.B) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, err := NewCrypto(context.Background(), nil, wallet, stubPrices{"PUMP/EUR": 1}, NoopPublisher(), &stubSignal{})

	if err != nil {
		b.Fatal(err)
	}

	crypto.records["PUMP/EUR"] = symbolRecord{wins: 3}

	b.ReportAllocs()

	for b.Loop() {
		if crypto.boostConfidence("PUMP/EUR", 0.8) <= 0 {
			b.Fatal("expected boost")
		}
	}
}

func BenchmarkCryptoRankCandidates(b *testing.B) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, err := NewCrypto(context.Background(), nil, wallet, stubPrices{"PUMP/EUR": 1}, NoopPublisher(), &stubSignal{})

	if err != nil {
		b.Fatal(err)
	}

	batch := []engine.Measurement{
		{Type: engine.Pump, Regime: "pump", Pairs: []asset.Pair{{Wsname: "A/EUR"}}, Confidence: 0.8},
		{Type: engine.Momentum, Regime: "momentum", Pairs: []asset.Pair{{Wsname: "B/EUR"}}, Confidence: 0.6},
		{Type: engine.Flow, Regime: "flow", Reason: "accumulation", Pairs: []asset.Pair{{Wsname: "C/EUR"}}, Confidence: 0.5},
	}

	b.ReportAllocs()

	for b.Loop() {
		if len(crypto.rankCandidates(batch)) == 0 {
			b.Fatal("expected candidates")
		}
	}
}
