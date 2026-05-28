package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/price"
	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCryptoHandleExitSuppressesSoftReasonsDuringMinimumHold(t *testing.T) {
	Convey("Given a fresh position and a soft exit", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		t.Cleanup(func() { pool.Close() })

		forecasts := price.NewPrediction(ctx, pool)
		t.Cleanup(func() { _ = forecasts.Close() })

		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
		tradingWallet.AddInventoryWithCost("BTC", 1, 100)
		tradingWallet.BindPosition("BTC", wallet.PositionBinding{
			Source:      "perspective:microstructure",
			PredictedAt: time.Now(),
			DueAt:       time.Now().Add(time.Minute),
		})
		crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
		t.Cleanup(func() { _ = crypto.Close() })

		err := crypto.handleExit(engine.Exit{
			Symbol:  "BTC/EUR",
			Urgency: 1,
			Reason:  engine.ExitReasonPressureFade,
		})

		Convey("It should keep the position open", func() {
			So(err, ShouldBeNil)
			So(tradingWallet.InventoryQty("BTC"), ShouldEqual, 1)
		})
	})
}

func TestCryptoHandleExitUsesStopLimitPrice(t *testing.T) {
	Convey("Given a stop hit while the cached bid is above the trigger", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		t.Cleanup(func() { pool.Close() })

		forecasts := price.NewPrediction(ctx, pool)
		t.Cleanup(func() { _ = forecasts.Close() })

		go func() {
			_ = forecasts.Tick()
		}()

		pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
			Value: market.TickerRow{
				Symbol: "BTC/EUR",
				Last:   100,
				Bid:    100,
				Ask:    100.2,
			},
		})

		waitForQuote(t, forecasts, "BTC/EUR")

		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 0, 0.26)
		tradingWallet.AddInventoryWithCost("BTC", 1, 100)
		tradingWallet.BindPosition("BTC", wallet.PositionBinding{
			Source:      "perspective:microstructure",
			PredictedAt: time.Now().Add(-config.System.MinExhaustHold),
			DueAt:       time.Now().Add(time.Minute),
		})
		crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
		t.Cleanup(func() { _ = crypto.Close() })

		err := crypto.handleExit(engine.Exit{
			Symbol:     "BTC/EUR",
			Urgency:    1,
			Reason:     engine.ExitReasonStopHit,
			LimitPrice: 99,
		})

		Convey("It should fill no higher than the stop trigger", func() {
			So(err, ShouldBeNil)
			So(tradingWallet.InventoryQty("BTC"), ShouldEqual, 0)
			So(tradingWallet.BalanceCopy(), ShouldBeLessThan, 99)
		})
	})
}

func waitForQuote(t *testing.T, forecasts *price.Prediction, symbol string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		last, _, _, _, ok := forecasts.LastQuote(symbol)

		if ok && last > 0 {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for quote %s", symbol)
}
