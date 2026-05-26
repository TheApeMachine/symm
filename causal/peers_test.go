package causal

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestPeerLinksRanksCorrelatedSymbols(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	causal := NewCausal(ctx, pool)
	t.Cleanup(func() { _ = causal.Close() })

	causal.symbols["BTC/EUR"] = NewCausalSymbol(asset.Pair{Wsname: "BTC/EUR"}, engine.DefaultCalibrationParams())
	causal.symbols["ETH/EUR"] = NewCausalSymbol(asset.Pair{Wsname: "ETH/EUR"}, engine.DefaultCalibrationParams())
	causal.symbols["SOL/EUR"] = NewCausalSymbol(asset.Pair{Wsname: "SOL/EUR"}, engine.DefaultCalibrationParams())

	for index := 0; index < minPeerSamples; index++ {
		causal.symbols["BTC/EUR"].samples = append(causal.symbols["BTC/EUR"].samples, causalSample{
			priceVelocity: float64(index) * 0.1,
		})
		causal.symbols["ETH/EUR"].samples = append(causal.symbols["ETH/EUR"].samples, causalSample{
			priceVelocity: float64(index) * 0.1,
		})
		causal.symbols["SOL/EUR"].samples = append(causal.symbols["SOL/EUR"].samples, causalSample{
			priceVelocity: float64(index%3) * 0.01,
		})
	}

	links := causal.peerLinks("BTC/EUR")

	if len(links) == 0 {
		t.Fatal("expected peer links")
	}

	first, ok := links[0]["symbol"].(string)

	if !ok || first != "ETH/EUR" {
		t.Fatalf("expected ETH/EUR as top peer, got %v", links[0])
	}

	correlation, ok := links[0]["correlation"].(float64)

	if !ok || correlation <= 0.99 {
		t.Fatalf("expected strong ETH correlation, got %v", links[0]["correlation"])
	}
}

func newTestPool(t *testing.T) *qpool.Q {
	t.Helper()

	pool := qpool.NewQ(context.Background(), 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	return pool
}
