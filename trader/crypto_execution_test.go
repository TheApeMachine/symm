package trader

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/wallet"
)

func TestCryptoExitSkipsPendingExit(t *testing.T) {
	convey.Convey("Given a held position with a paper exit already queued", t, func() {
		crypto := newTestCrypto()
		symbol := "ALGO/EUR"
		base := "ALGO"
		now := time.Now()

		crypto.wallet.AddInventory(base, 1, 100)
		crypto.wallet.BindPosition(base, wallet.PositionBinding{
			Source:      "perspective",
			Playbook:    string(perspectives.PlaybookLeadLag),
			PredictedAt: now,
			DueAt:       now.Add(time.Minute),
			TakerFeePct: 0.26,
		})
		crypto.positions.Open(symbol, positionState{
			Playbook: string(perspectives.PlaybookLeadLag),
			Peak:     100,
			EntryAt:  now,
		})
		crypto.open.Store(1)
		seedExitQuote(crypto, symbol, 100)

		crypto.exit(symbol, 100, perspectives.ActionTakeProfit, "first exit")
		crypto.exit(symbol, 99, perspectives.ActionTakeProfit, "second exit")

		convey.Convey("It should queue only one sell fill until the first exit resolves", func() {
			convey.So(crypto.paper.HasPendingExit(symbol), convey.ShouldBeTrue)
			convey.So(len(crypto.paper.fills), convey.ShouldEqual, 1)

			drainOrderEvents(crypto)
			_, held := crypto.wallet.PositionBindingFor(base)

			convey.So(crypto.paper.HasPendingExit(symbol), convey.ShouldBeFalse)
			convey.So(held, convey.ShouldBeFalse)
			convey.So(crypto.wallet.InventoryQty(base), convey.ShouldEqual, 0)
			convey.So(crypto.open.Load(), convey.ShouldEqual, 0)
			convey.So(crypto.wallet.BalanceCopy(), convey.ShouldBeLessThan, 350)
		})
	})
}

func TestApplySellFillRejectsPaperOversell(t *testing.T) {
	convey.Convey("Given a paper sell fill larger than wallet inventory", t, func() {
		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
		tradingWallet.AddInventory("BTC", 1, 100)

		err := applySellFill(tradingWallet, order.Fill{
			Symbol:  "BTC/EUR",
			Side:    "sell",
			Qty:     2,
			Price:   100,
			FeeCcy:  "EUR",
			ExecKey: "paper-oversell",
		})

		convey.Convey("It should fail closed without crediting phantom cash", func() {
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(tradingWallet.InventoryQty("BTC"), convey.ShouldEqual, 1)
			convey.So(tradingWallet.BalanceCopy(), convey.ShouldEqual, 200)
		})
	})
}

func TestHandleExitFillPublishesFillOnce(t *testing.T) {
	convey.Convey("Given an exit fill published to the UI bus", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		crypto := newTestCrypto()
		crypto.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
		subscription := crypto.ui.Subscribe("test:exit-fill", 16)
		now := time.Now()

		crypto.wallet.AddInventory("BTC", 0.01, 100)
		crypto.wallet.BindPosition("BTC", wallet.PositionBinding{
			Source:      "perspective",
			Playbook:    string(perspectives.PlaybookLeadLag),
			PredictedAt: now,
			DueAt:       now.Add(time.Minute),
		})

		crypto.handleExitFill(order.Fill{
			ClOrdID: "exit-1",
			Symbol:  "BTC/EUR",
			Side:    "sell",
			Qty:     0.01,
			Price:   100,
			FeeCcy:  "EUR",
			ExecKey: "paper-exit-1",
		}, orderIntent{
			kind:        "exit",
			symbol:      "BTC/EUR",
			playbook:    string(perspectives.PlaybookLeadLag),
			entryPrice:  100,
			exitReason:  "test exit",
			predictedAt: now,
		})

		fillCount := 0
		deadline := time.After(200 * time.Millisecond)
		collecting := true

		for collecting {
			select {
			case value := <-subscription.Incoming:
				if _, ok := value.Value.(order.Fill); ok {
					fillCount++
				}
			case <-deadline:
				collecting = false
			}
		}

		convey.Convey("It should publish exactly one fill frame", func() {
			convey.So(fillCount, convey.ShouldEqual, 1)
		})
	})
}
