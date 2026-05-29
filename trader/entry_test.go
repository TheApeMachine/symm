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

	. "github.com/smartystreets/goconvey/convey"
)

func TestEntryFrictionReturn(t *testing.T) {
	Convey("Given a taker paper entry with a quoted spread", t, func() {
		originalUseMaker := config.System.UseMakerEntries
		originalTaker := config.System.TakerFeePct
		config.System.UseMakerEntries = false
		config.System.TakerFeePct = 0.26
		t.Cleanup(func() {
			config.System.UseMakerEntries = originalUseMaker
			config.System.TakerFeePct = originalTaker
		})

		// No Pairs on the measurement, so friction falls back to the configured
		// taker fee: 0.26% round-trip (×2) + 20 bps spread = 0.0072.
		friction := entryFrictionReturn(engine.Measurement{
			Last: 100,
			Bid:  99.9,
			Ask:  100.1,
		})

		Convey("It should include taker round-trip fees and full spread", func() {
			So(friction, ShouldAlmostEqual, 0.0072, 1e-9)
		})
	})

	Convey("Given a measurement whose pair carries a real fee schedule", t, func() {
		originalUseMaker := config.System.UseMakerEntries
		config.System.UseMakerEntries = false
		t.Cleanup(func() { config.System.UseMakerEntries = originalUseMaker })

		// Bottom-tier taker 0.40% round-trip (×2 = 0.008) + 20 bps spread = 0.010.
		friction := entryFrictionReturn(engine.Measurement{
			Last: 100,
			Bid:  99.9,
			Ask:  100.1,
			Pairs: []asset.Pair{{
				Wsname: "BTC/EUR",
				Fees:   [][]float64{{0, 0.4}, {10000, 0.35}},
			}},
		})

		Convey("It should use the per-pair taker fee, not the config fallback", func() {
			So(friction, ShouldAlmostEqual, 0.010, 1e-9)
		})
	})
}

func TestCryptoTryEnterHonorsEnterVerdict(t *testing.T) {
	Convey("Given the decision tree returns ALLOW ENTRY", t, func() {
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
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
			Confidence: 0.9,
			Last:       100,
			Bid:        99.9,
			Ask:        100.1,
		}
		prediction := engine.Prediction{
			Perspective: engine.Perspective{
				Type:         engine.PerspectiveMicrostructure,
				Measurements: []engine.Measurement{measurement},
			},
			Confidence:  0.9,
			PredictedAt: time.Now(),
			DueAt:       time.Now().Add(time.Minute),
		}

		// The tree authorized entry. A cold Kelly must not silently veto it: the
		// slot is floored at the minimum tradeable notional, so a position opens.
		crypto.tryEnter(prediction, 0.0, engine.Verdict{Action: engine.ActionEnter, Direction: 1, Node: "stage4_node8"})

		Convey("It should open a position", func() {
			So(tradingWallet.Inventory["BTC"], ShouldBeGreaterThan, 0)
		})
	})
}
