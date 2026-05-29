package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/price"
	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExecutionManagerHandleExit(t *testing.T) {
	Convey("Given a resting live entry", t, func() {
		crypto := testLiveCrypto(t)
		orders := crypto.broadcasts["orders"].Subscribe("test:orders", 8)

		So(crypto.wallet.ReserveEntry(10), ShouldBeNil)
		So(crypto.execution.Track(liveEntryOrder{
			Symbol:   "BTC/EUR",
			ClOrdID:  "CLIENT-1",
			OrderID:  "ORDER-1",
			Notional: 10,
			Reserved: 10,
		}), ShouldBeNil)

		handled := crypto.execution.HandleExit(engine.Exit{
			Symbol:  "BTC/EUR",
			Urgency: config.System.ExitUrgencyThreshold,
			Reason:  "book_thinning",
		})

		Convey("It should cancel the exchange order without releasing before ack", func() {
			So(handled, ShouldBeTrue)

			value := <-orders.Incoming
			request := value.Value.(order.CancelRequest)
			So(request.Method, ShouldEqual, order.MethodCancelOrder)
			So(request.Params.OrderID, ShouldEqual, "ORDER-1")
			So(crypto.wallet.ReservedCopy(), ShouldEqual, 10)
		})

		Convey("It should release unfilled reservation after cancel ack", func() {
			<-orders.Incoming
			ack := &order.Ack{Method: order.MethodCancelOrder, Success: true}
			ack.Result.OrderID = "ORDER-1"
			crypto.execution.HandleAck(ack)

			So(crypto.wallet.ReservedCopy(), ShouldEqual, 0)
			So(crypto.wallet.BalanceCopy(), ShouldEqual, 200)
		})
	})
}

func TestExecutionManagerReviewMeasurement(t *testing.T) {
	Convey("Given a stalled maker entry and robust flow confirmation", t, func() {
		original := *config.System
		config.System.ExecutionMakerFallbackTicks = 2
		t.Cleanup(func() { *config.System = original })

		crypto := testLiveCrypto(t)
		orders := crypto.broadcasts["orders"].Subscribe("test:fallback-orders", 8)

		So(crypto.wallet.ReserveEntry(10), ShouldBeNil)
		So(crypto.execution.Track(liveEntryOrder{
			Symbol:          "BTC/EUR",
			ClOrdID:         "CLIENT-1",
			OrderID:         "ORDER-1",
			Notional:        10,
			Reserved:        10,
			EntryConfidence: 0.5,
		}), ShouldBeNil)

		measurement := engine.Measurement{
			Source:     "cvd",
			Confidence: 0.6,
			Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		}

		crypto.execution.ReviewMeasurement(measurement)
		crypto.execution.ReviewMeasurement(measurement)

		Convey("It should request cancel before any taker fallback", func() {
			value := <-orders.Incoming
			So(value.Value.(order.CancelRequest).Method, ShouldEqual, order.MethodCancelOrder)
		})

		Convey("It should submit the taker fallback only after cancel ack", func() {
			<-orders.Incoming
			ack := &order.Ack{Method: order.MethodCancelOrder, Success: true}
			ack.Result.OrderID = "ORDER-1"
			crypto.execution.HandleAck(ack)

			value := <-orders.Incoming
			request := value.Value.(order.Request)
			So(request.Method, ShouldEqual, order.MethodAddOrder)
			So(request.Params.OrderType, ShouldEqual, order.Market)
			So(request.Params.CashOrderQty, ShouldEqual, 10)
		})
	})
}

func TestExecutionManagerHandleFill(t *testing.T) {
	Convey("Given a partial live buy fill", t, func() {
		crypto := testLiveCrypto(t)
		prediction := engine.Prediction{
			Perspective: engine.Perspective{Type: engine.PerspectiveMicrostructure},
			PredictedAt: time.Now(),
			DueAt:       time.Now().Add(time.Minute),
		}

		So(crypto.wallet.ReserveEntry(10), ShouldBeNil)
		So(crypto.execution.Track(liveEntryOrder{
			Symbol:     "BTC/EUR",
			ClOrdID:    "CLIENT-1",
			OrderID:    "ORDER-1",
			Notional:   10,
			Reserved:   10,
			Prediction: prediction,
		}), ShouldBeNil)

		crypto.applyFill(order.Fill{
			OrderID: "ORDER-1",
			ClOrdID: "CLIENT-1",
			Symbol:  "BTC/EUR",
			Side:    "buy",
			Qty:     0.01,
			Price:   100,
			ExecKey: "EXEC-1",
		})

		Convey("It should settle only the filled reservation and bind the position", func() {
			So(crypto.wallet.ReservedCopy(), ShouldEqual, 9)
			So(crypto.wallet.InventoryQty("BTC"), ShouldEqual, 0.01)
			_, ok := crypto.wallet.PositionBindingFor("BTC")
			So(ok, ShouldBeTrue)
		})
	})
}

func BenchmarkExecutionManagerHandleFill(b *testing.B) {
	crypto := testLiveCrypto(b)
	fill := order.Fill{
		OrderID: "ORDER-BENCH",
		ClOrdID: "CLIENT-BENCH",
		Symbol:  "BTC/EUR",
		Side:    "buy",
		Qty:     0.01,
		Price:   100,
	}

	for b.Loop() {
		crypto.wallet = wallet.NewWallet(wallet.CryptoWallet, "EUR", 200, config.System.TakerFeePct)
		crypto.execution = newExecutionManager(crypto)
		_ = crypto.wallet.ReserveEntry(10)
		_ = crypto.execution.Track(liveEntryOrder{
			Symbol:   "BTC/EUR",
			ClOrdID:  "CLIENT-BENCH",
			OrderID:  "ORDER-BENCH",
			Notional: 10,
			Reserved: 10,
		})
		crypto.wallet.ApplyFill("", fill.Side, "BTC", fill.Qty, fill.Price, -fill.Qty*fill.Price)
		crypto.execution.HandleFill(fill)
	}
}

type liveCryptoTest interface {
	Cleanup(func())
}

func testLiveCrypto(testingT liveCryptoTest) *Crypto {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	testingT.Cleanup(func() { pool.Close() })

	forecasts := price.NewPrediction(ctx, pool)
	testingT.Cleanup(func() { _ = forecasts.Close() })

	tradingWallet := wallet.NewWallet(wallet.CryptoWallet, "EUR", 200, config.System.TakerFeePct)
	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	testingT.Cleanup(func() { _ = crypto.Close() })

	return crypto
}
