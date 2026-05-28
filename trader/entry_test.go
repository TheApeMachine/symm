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
		config.System.UseMakerEntries = false
		t.Cleanup(func() { config.System.UseMakerEntries = originalUseMaker })

		friction := entryFrictionReturn(engine.Measurement{
			Last: 100,
			Bid:  99.9,
			Ask:  100.1,
		})

		Convey("It should include taker round-trip fees and full spread", func() {
			So(friction, ShouldAlmostEqual, 0.0072, 1e-9)
		})
	})
}

func TestCryptoTryEnterRequiresEdgeMultiple(t *testing.T) {
	Convey("Given a prediction below the required entry edge multiple", t, func() {
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
		predictedReturn := entryFrictionReturn(measurement) * 1.5

		crypto.tryEnter(prediction, predictedReturn)

		Convey("It should not open inventory", func() {
			So(tradingWallet.Inventory["BTC"], ShouldEqual, 0)
		})
	})
}
