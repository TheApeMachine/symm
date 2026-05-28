package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
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

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	t.Cleanup(func() { _ = crypto.Close() })

	// Warm the calibrator: no-cold-trading policy means entries only fire
	// after MinCalibrationSamples settlements exist for this (source, regime).
	source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)
	regime := engine.CalibrationRegime(engine.FeedbackRegime(
		engine.Perspective{Type: engine.PerspectiveMicrostructure},
		engine.Measurement{Regime: "cluster"},
	))
	for range config.System.MinCalibrationSamples + 1 {
		crypto.kellySizer.ApplyFeedback(engine.PredictionFeedback{
			Source:          source,
			Symbol:          "BTC/EUR",
			Regime:          regime,
			PredictedReturn: 0.01,
			ActualReturn:    0.015,
		})
	}

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

/*
TestCryptoColdStartDoesNotEnter guards the no-cold-trading policy at the
trader level. A measurement that would have triggered an entry under the
old MaxFraction fallback must now produce zero inventory until the
calibrator has settled enough samples to size it.
*/
func TestCryptoColdStartDoesNotEnter(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	forecasts := price.NewPrediction(ctx, pool)
	t.Cleanup(func() { _ = forecasts.Close() })

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	t.Cleanup(func() { _ = crypto.Close() })

	measurement := engine.Measurement{
		Type:       engine.Momentum,
		Source:     "hawkes",
		Regime:     "cluster",
		Reason:     "burst",
		Pairs:      []asset.Pair{{Wsname: "ETH/EUR"}},
		Confidence: 0.85,
		Last:       3000,
		Bid:        2999,
		Ask:        3001,
	}

	if err := crypto.ingestMeasurement(measurement); err != nil {
		t.Fatalf("ingest measurement: %v", err)
	}

	if tradingWallet.Inventory["ETH"] > 0 {
		t.Fatalf("expected cold start to skip entry, got %v ETH", tradingWallet.Inventory["ETH"])
	}
}

// TestSettlePredictionsDoesNotApplyFeedbackLocally guards against
// double-counting: settlePredictions must publish on the "feedback" channel
// only, leaving ApplyFeedback to the trader's subscriber loop (or any other
// subscriber). Otherwise the calibrator slot receives the same feedback twice.
func TestSettlePredictionsDoesNotApplyFeedbackLocally(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	forecasts := price.NewPrediction(ctx, pool)
	t.Cleanup(func() { _ = forecasts.Close() })

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	t.Cleanup(func() { _ = crypto.Close() })

	before := 0.0
	if stats := crypto.kellySizer.bySeries[sourceSlotKey{source: "hawkes", regime: engine.CalibrationRegime("cluster")}]; stats != nil {
		before = stats.wins.Total()
	}

	due := &engine.Prediction{
		Perspective: engine.Perspective{
			Type: engine.PerspectiveMicrostructure,
			Measurements: []engine.Measurement{{
				Source:     "hawkes",
				Regime:     "cluster",
				Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
				Confidence: 0.8,
				Last:       100,
				Bid:        99.9,
				Ask:        100.1,
			}},
		},
		Confidence:     0.8,
		Direction:      1,
		ExpectedReturn: 0.01,
		ActualReturn:   0.012,
		Runway:         time.Second,
		PredictedAt:    time.Now().Add(-2 * time.Second),
		DueAt:          time.Now().Add(-time.Second),
	}

	crypto.predictions = append(crypto.predictions, due)
	crypto.settlePredictions(engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Last:  101.2,
	})

	stats := crypto.kellySizer.bySeries[sourceSlotKey{
		source: engine.PerspectiveSource(engine.PerspectiveMicrostructure),
		regime: engine.CalibrationRegime(engine.FeedbackRegime(due.Perspective, due.Perspective.Measurements[0])),
	}]

	if stats != nil && stats.wins.Total() > before {
		t.Fatalf("settlePredictions must not call ApplyFeedback directly; wins moved by %v",
			stats.wins.Total()-before)
	}
}

func TestSettlePredictionsDoesNotExitUnboundPosition(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	forecasts := price.NewPrediction(ctx, pool)
	t.Cleanup(func() { _ = forecasts.Close() })

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	tradingWallet.Inventory["BTC"] = 0.1
	tradingWallet.AvgEntry["BTC"] = 100

	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	t.Cleanup(func() { _ = crypto.Close() })

	now := time.Now()
	activeDue := now.Add(10 * time.Minute)
	tradingWallet.BindPosition("BTC", wallet.PositionBinding{
		Source:      engine.PerspectiveSource(engine.PerspectiveFlow),
		PredictedAt: now.Add(-time.Minute),
		DueAt:       activeDue,
	})

	due := settledTestPrediction(
		engine.PerspectiveMicrostructure,
		now.Add(-2*time.Second),
	)
	crypto.predictions = append(crypto.predictions, due)

	crypto.settlePredictions(engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Last:  101,
	})

	if tradingWallet.InventoryQty("BTC") <= config.System.LiveInventoryEpsilon {
		t.Fatal("observational prediction settlement closed an unrelated position")
	}
}

func TestSettlePredictionsExitsBoundPosition(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	forecasts := price.NewPrediction(ctx, pool)
	t.Cleanup(func() { _ = forecasts.Close() })

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	tradingWallet.Inventory["BTC"] = 0.1
	tradingWallet.AvgEntry["BTC"] = 100

	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	t.Cleanup(func() { _ = crypto.Close() })

	now := time.Now()
	due := settledTestPrediction(
		engine.PerspectiveMicrostructure,
		now.Add(-2*time.Second),
	)
	tradingWallet.BindPosition("BTC", wallet.PositionBinding{
		Source:      engine.PerspectiveSource(due.Perspective.Type),
		PredictedAt: due.PredictedAt,
		DueAt:       due.DueAt,
	})
	crypto.predictions = append(crypto.predictions, due)

	crypto.settlePredictions(engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
		Last:  101,
	})

	if tradingWallet.InventoryQty("BTC") > config.System.LiveInventoryEpsilon {
		t.Fatalf("expected bound position closed, got %v", tradingWallet.InventoryQty("BTC"))
	}
}

func settledTestPrediction(
	perspectiveType engine.PerspectiveType,
	dueAt time.Time,
) *engine.Prediction {
	return &engine.Prediction{
		Perspective: engine.Perspective{
			Type: perspectiveType,
			Measurements: []engine.Measurement{{
				Source:     "hawkes",
				Regime:     "cluster",
				Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
				Confidence: 0.8,
				Last:       100,
				Bid:        99.9,
				Ask:        100.1,
			}},
		},
		Confidence:     0.8,
		Direction:      1,
		ExpectedReturn: 0.01,
		Runway:         time.Second,
		PredictedAt:    dueAt.Add(-time.Second),
		DueAt:          dueAt,
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
