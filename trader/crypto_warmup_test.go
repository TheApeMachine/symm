package trader

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestReadyForTradingRequiresQuoteCoverageAndWarmPulses(t *testing.T) {
	crypto := &Crypto{
		engineStats: stubEngineStats{
			tickerReady: func() int { return 90 },
			symbolTotal: func() int { return 100 },
		},
	}

	if crypto.readyForTrading() {
		t.Fatal("expected warm-up to block trading without pulses")
	}

	crypto.pulseSeq.Store(int64(config.System.MinWarmPulses))

	if crypto.readyForTrading() {
		t.Fatal("expected insufficient quote coverage to block trading")
	}

	crypto.engineStats = stubEngineStats{
		tickerReady: func() int { return 96 },
		symbolTotal: func() int { return 100 },
	}
	crypto.pulseSeq.Store(int64(config.System.MinWarmPulses))

	if !crypto.readyForTrading() {
		t.Fatal("expected ready state once coverage and pulses are satisfied")
	}
}

func TestDecideRequiresConsecutiveCandidate(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	prices := stubPrices{"PUMP/EUR": 1.0}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), &stubSignal{})
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	batch := []engine.Measurement{{
		Type:       engine.Momentum,
		Source:     "hawkes",
		Regime:     "momentum",
		Reason:     "cluster_buy",
		Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
		Confidence: 0.8,
	}}

	primeDecideCrypto(crypto)
	crypto.decide(batch)

	if len(crypto.holds) != 0 {
		t.Fatalf("expected first candidate sighting to be ignored, holds=%d", len(crypto.holds))
	}

	crypto.decide(batch)

	if len(crypto.holds) != 1 {
		t.Fatalf("expected entry after consecutive candidate, holds=%d", len(crypto.holds))
	}
}

func TestDecideBlocksDuringWarmPhase(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	prices := stubPrices{"PUMP/EUR": 1.0}

	crypto, err := NewCrypto(context.Background(), nil, wallet, prices, NoopPublisher(), &stubSignal{})
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.engineStats = stubEngineStats{
		tickerReady: func() int { return 10 },
		symbolTotal: func() int { return 100 },
	}

	batch := []engine.Measurement{{
		Type:       engine.Momentum,
		Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
		Confidence: 0.8,
	}}

	for index := 0; index < 3; index++ {
		crypto.decide(batch)
	}

	if len(crypto.holds) != 0 {
		t.Fatalf("expected warm phase to block entries, holds=%d", len(crypto.holds))
	}

	if crypto.pulseSeq.Load() == 0 {
		t.Fatal("expected decide to advance pulse sequence during warm phase")
	}
}
