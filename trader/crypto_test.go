package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/price"
	"github.com/theapemachine/symm/wallet"
)

func TestCryptoEnterAndExit(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	forecasts := price.NewPrediction(ctx, pool)
	t.Cleanup(func() { _ = forecasts.Close() })

	forecasts.SeedReturnCalibration(
		engine.PerspectiveSource(engine.PerspectiveMicrostructure),
		"BTC/EUR",
		0.02,
	)

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	t.Cleanup(func() { _ = crypto.Close() })

	measurement := engine.Measurement{
		Type:       engine.Momentum,
		Source:     "hawkes",
		Regime:     "cluster",
		Reason:     "burst",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.9,
		Last:       100,
		Bid:        99.9,
		Ask:        100.1,
		Timeframe: engine.Timeframe{
			Start: time.Now().Add(-30 * time.Second).Unix(),
			End:   time.Now().Add(30 * time.Second).Unix(),
		},
	}

	if err := crypto.ingestMeasurement(measurement); err != nil {
		t.Fatalf("ingest measurement: %v", err)
	}

	if tradingWallet.Inventory["BTC"] <= 0 {
		t.Fatal("expected paper entry to open BTC inventory")
	}

	if err := crypto.handleExit(engine.Exit{
		Symbol:  "BTC/EUR",
		Urgency: 0.9,
		Reason:  engine.ExitReasonPressureFade,
	}); err != nil {
		t.Fatalf("handle exit: %v", err)
	}

	if tradingWallet.Inventory["BTC"] > 0 {
		t.Fatalf("expected BTC inventory cleared after exit, got %v", tradingWallet.Inventory["BTC"])
	}
}

func TestNewPerspectiveSeedsRegimes(t *testing.T) {
	measurement := engine.Measurement{
		Source:     "hawkes",
		Regime:     "cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.8,
		Last:       100,
	}

	perspective := NewPerspective([]engine.Measurement{measurement})

	if len(perspective.regimes) == 0 {
		t.Fatal("expected regimes to be seeded from initial measurements")
	}
}
