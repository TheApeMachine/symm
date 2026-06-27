package broker

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func TestDeskOpensAndArmsStop(testingTB *testing.T) {
	Convey("Given a live ticker mark and a buy action", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"type":      "market",
				"quantity":  0.1,
				"cl_ord_id": "open-1",
				"offset":    0.05,
			}.Marshal())

		So(desk.Update([]*datura.Artifact{action}), ShouldBeNil)

		Convey("It sends a buy order and waits for the fill before arming the stop", func() {
			order := awaitOrder(orders)

			So(order, ShouldNotBeNil)
			So(datura.Peek[string](order, "params", "side"), ShouldEqual, "buy")
			So(datura.Peek[string](order, "params", "symbol"), ShouldEqual, "BTC/USD")
			So(datura.Peek[float64](order, "params", "order_qty"), ShouldEqual, 0.1)
			So(loadStop(tree, "BTC/USD"), ShouldBeNil)

			seedExecution(desk, "open-1", "BTC/USD", "buy", 0.1, 101)
			stop := loadStop(tree, "BTC/USD")

			So(stop, ShouldNotBeNil)
			snapshot := stop.Snapshot()
			So(snapshot.Qty, ShouldEqual, 0.1)
			So(snapshot.Peak, ShouldEqual, 101)
			So(snapshot.Stop, ShouldAlmostEqual, 95.95, 1e-9)
		})
	})
}

func TestDeskOnMessageRejectsInvalidArtifacts(testingTB *testing.T) {
	Convey("Given a desk", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")
		desk := NewDesk(ctx, pool, tree)

		Convey("Nil artifacts should return errors instead of panicking", func() {
			So(desk.onMessage(nil), ShouldNotBeNil)
		})

		Convey("Artifacts missing routing metadata should return errors instead of panicking", func() {
			artifact := datura.Acquire("test", datura.APPJSON)
			So(desk.onMessage(artifact), ShouldNotBeNil)
		})
	})
}

func TestDeskCloseStopsMessageProcessing(testingTB *testing.T) {
	Convey("Given a closed desk still registered with the ticker broadcast group", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		So(desk.Close(), ShouldBeNil)

		frame := `{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99,"ask":101}]}`
		artifact := datura.Acquire("websocket", datura.APPJSON).
			WithDestination("desk").
			WithRole("ticker").
			WithScope("update").
			WithPayload([]byte(frame))

		err := pool.CreateBroadcastGroup("ticker").Send(artifact)

		Convey("The closed callback should not mutate quote state", func() {
			So(err, ShouldBeNil)
			So(desk.Update(nil), ShouldNotBeNil)
			_, ok := desk.snapshot().quotes["BTC/USD"]
			So(ok, ShouldBeFalse)
		})
	})
}

func TestDeskRatchetsStopUp(testingTB *testing.T) {
	Convey("Given an armed stop and a higher mark", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		storeStop(desk, NewStoploss("BTC/USD", 0.1, 100, 0.05))
		seedTicker(desk, "BTC/USD", 120)

		So(desk.Update(nil), ShouldBeNil)

		Convey("The stop trails up and the position stays open", func() {
			stop := loadStop(tree, "BTC/USD")
			snapshot := stop.Snapshot()

			So(snapshot.Qty, ShouldEqual, 0.1)
			So(snapshot.Peak, ShouldEqual, 120)
			So(snapshot.Stop, ShouldAlmostEqual, 114, 1e-9)
		})
	})
}

func TestDeskRestoresOpenStopsOnStartup(testingTB *testing.T) {
	Convey("Given stoploss artifacts already stored in the tree", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		active := NewStoploss("BTC/USD", 0.1, 100, 0.05)
		closed := NewStoploss("ETH/USD", 0.2, 50, 0.05)
		So(closed.Close(), ShouldBeNil)

		_, _, activeErr := tree.InsertArtifact(stoplossKey(active.Symbol), active.Artifact())
		So(activeErr, ShouldBeNil)
		_, _, closedErr := tree.InsertArtifact(stoplossKey(closed.Symbol), closed.Artifact())
		So(closedErr, ShouldBeNil)

		desk := NewDesk(ctx, pool, tree)

		Convey("NewDesk should restore only open stops", func() {
			state := desk.snapshot()

			So(state.stoplosses["BTC/USD"], ShouldNotBeNil)
			So(state.stoplosses["ETH/USD"], ShouldBeNil)
			So(state.stoplosses["BTC/USD"].Snapshot().Qty, ShouldEqual, 0.1)
		})

		Convey("NewDesk should publish a diagnostic listing restored stops", func() {
			var diagnostic *datura.Artifact

			for artifact := range tree.Seek([]byte("diagnostic/stop_restore")) {
				diagnostic = artifact
				break
			}

			So(diagnostic, ShouldNotBeNil)
			So(datura.Peek[string](diagnostic, "code"), ShouldEqual, "stop_restore")
			So(datura.Peek[string](diagnostic, "context", "symbols", 0), ShouldEqual, "BTC/USD")
		})
	})
}

func TestDeskUsesBidForStopRatchetWhenLastIsZero(testingTB *testing.T) {
	Convey("Given an armed stop and a ticker row with a zero last trade", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		storeStop(desk, NewStoploss("OMNI/USD", 0.1, 100, 0.05))
		seedTickerQuote(desk, "OMNI/USD", 0, 120, 121)

		So(desk.Update(nil), ShouldBeNil)

		Convey("The stop trails against the executable bid", func() {
			stop := loadStop(tree, "OMNI/USD")
			snapshot := stop.Snapshot()

			So(snapshot.Qty, ShouldEqual, 0.1)
			So(snapshot.Peak, ShouldEqual, 120)
			So(snapshot.Stop, ShouldAlmostEqual, 114, 1e-9)
		})
	})
}

func TestDeskAllowsZeroLastForUnheldTickerRow(testingTB *testing.T) {
	Convey("Given an unrelated ticker row with a zero last trade", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		seedTickerQuote(desk, "OMNI/USD", 0, 1.2, 1.3)

		Convey("It should not treat last trade as the broker mark invariant", func() {
			So(desk.Update(nil), ShouldBeNil)
		})
	})
}

func TestDeskRetainsQuotesAcrossPartialTickerUpdates(testingTB *testing.T) {
	Convey("Given ticker updates arrive as partial symbol frames", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		seedTickerQuote(desk, "BTC/USD", 100, 99, 101)
		seedTickerQuote(desk, "ETH/USD", 50, 49, 51)

		Convey("The later ETH update should not erase the BTC quote", func() {
			mark, err := desk.markFor("BTC/USD")

			So(err, ShouldBeNil)
			So(mark, ShouldEqual, 101)
		})
	})
}

func TestDeskExitsOnStopBreach(testingTB *testing.T) {
	Convey("Given an armed stop and a mark below it", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)
		storeStop(desk, NewStoploss("BTC/USD", 0.1, 100, 0.05))
		seedTicker(desk, "BTC/USD", 90)

		So(desk.Update(nil), ShouldBeNil)

		Convey("The desk sends a protective sell and leaves the stop active until fill confirmation", func() {
			order := awaitOrder(orders)

			So(order, ShouldNotBeNil)
			So(datura.Peek[string](order, "params", "side"), ShouldEqual, "sell")
			So(datura.Peek[float64](order, "params", "order_qty"), ShouldEqual, 0.1)
			So(datura.Peek[string](order, "params", "cl_ord_id"), ShouldStartWith, "stop-")

			stop := loadStop(tree, "BTC/USD")
			snapshot := stop.Snapshot()

			So(snapshot.Qty, ShouldEqual, 0.1)
			So(len(desk.snapshot().orders), ShouldEqual, 1)
		})
	})
}

func TestDeskRejectsEntryWithoutOffset(testingTB *testing.T) {
	Convey("Given a buy action with no stop offset", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"type":     "market",
				"quantity": 0.1,
			}.Marshal())

		updateErr := desk.Update([]*datura.Artifact{action})

		Convey("It should raise an error instead of continuing unprotected", func() {
			So(updateErr, ShouldNotBeNil)
			So(loadStop(tree, "BTC/USD"), ShouldBeNil)
		})
	})
}

func TestDeskRejectsMissingOrderType(testingTB *testing.T) {
	Convey("Given a buy action with no order type", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"quantity": 0.1,
				"offset":   0.05,
			}.Marshal())

		updateErr := desk.Update([]*datura.Artifact{action})

		Convey("It should fail before creating pending entry state", func() {
			So(updateErr, ShouldNotBeNil)
			So(len(desk.snapshot().pending), ShouldEqual, 0)
		})
	})
}

func TestDeskRejectsUnsupportedOrderType(testingTB *testing.T) {
	Convey("Given a buy action with an unsupported order type", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"type":     "iceberg",
				"quantity": 0.1,
				"offset":   0.05,
			}.Marshal())

		updateErr := desk.Update([]*datura.Artifact{action})

		Convey("It should fail before creating pending entry state", func() {
			So(updateErr, ShouldNotBeNil)
			So(len(desk.snapshot().pending), ShouldEqual, 0)
		})
	})
}

func TestDeskRequiresFillConfirmationBeforeArmingStop(testingTB *testing.T) {
	Convey("Given a pending buy and a non-fill execution update", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"type":      "market",
				"quantity":  0.1,
				"cl_ord_id": "open-1",
				"offset":    0.05,
			}.Marshal())

		So(desk.Update([]*datura.Artifact{action}), ShouldBeNil)
		So(awaitOrder(orders), ShouldNotBeNil)
		So(desk.snapshot().pending["open-1"].Symbol, ShouldEqual, "BTC/USD")
		So(desk.snapshot().orders["open-1"].Symbol, ShouldEqual, "BTC/USD")

		err := desk.observeExecutions(executionArtifact(
			"open-1", "BTC/USD", "buy", 0.1, 0.1, 101, "open", "accepted",
		))

		Convey("It should acknowledge the order but leave the position unarmed", func() {
			So(err, ShouldBeNil)
			So(loadStop(tree, "BTC/USD"), ShouldBeNil)
			So(desk.snapshot().pending["open-1"].Symbol, ShouldEqual, "BTC/USD")
			So(desk.snapshot().orders["open-1"].Symbol, ShouldEqual, "")
		})
	})
}

func TestDeskEntryAckTimeoutClearsPendingEntry(testingTB *testing.T) {
	Convey("Given a buy order that never receives an acknowledgement", testingTB, func() {
		viper.Set("trading.order_ack_timeout", time.Second)
		defer viper.Reset()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"type":      "market",
				"quantity":  0.1,
				"cl_ord_id": "open-timeout",
				"offset":    0.05,
			}.Marshal())

		So(desk.Update([]*datura.Artifact{action}), ShouldBeNil)
		So(awaitOrder(orders), ShouldNotBeNil)
		So(desk.snapshot().pending["open-timeout"].Symbol, ShouldEqual, "BTC/USD")
		So(desk.snapshot().orders["open-timeout"].Symbol, ShouldEqual, "BTC/USD")

		err := desk.reconcileOrderAcks(time.Now().UTC().Add(2 * time.Second))

		Convey("It should remove the unprotected pending entry and publish a diagnostic", func() {
			So(err, ShouldBeNil)
			So(desk.snapshot().pending["open-timeout"].Symbol, ShouldEqual, "")
			So(desk.snapshot().orders["open-timeout"].Symbol, ShouldEqual, "")
			So(diagnosticExists(tree, "order_ack_timeout"), ShouldBeTrue)
		})
	})
}

func TestDeskProtectiveExitAckTimeoutKeepsStopActive(testingTB *testing.T) {
	Convey("Given a protective sell order that never receives an acknowledgement", testingTB, func() {
		viper.Set("trading.order_ack_timeout", time.Second)
		defer viper.Reset()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		seedInstrument(tree, "BTC/USD", `{"qty_increment":0.000000000001,"qty_min":0.00000001,"cost_min":0.01}`)

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)
		storeStop(desk, NewStoploss("BTC/USD", 0.1, 100, 0.05))

		So(desk.Update([]*datura.Artifact{sellAction("exit-timeout", 0.1)}), ShouldBeNil)
		So(awaitOrder(orders), ShouldNotBeNil)
		So(desk.snapshot().orders["exit-timeout"].Symbol, ShouldEqual, "BTC/USD")

		err := desk.reconcileOrderAcks(time.Now().UTC().Add(2 * time.Second))

		Convey("It should alert but keep the stop and tracked exit active", func() {
			So(err, ShouldBeNil)
			So(loadStop(tree, "BTC/USD").Snapshot().Qty, ShouldEqual, 0.1)
			So(desk.snapshot().orders["exit-timeout"].TimeoutNotified, ShouldBeTrue)
			So(diagnosticExists(tree, "order_ack_timeout"), ShouldBeTrue)
		})
	})
}

func TestDeskSellFillRetiresStop(testingTB *testing.T) {
	Convey("Given a sell fill for an active stop", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		desk := NewDesk(ctx, pool, tree)
		storeStop(desk, NewStoploss("BTC/USD", 0.1, 100, 0.05))
		desk.trackOrder("BTC/USD", "sell", "exit-1", time.Now().UTC())

		err := desk.onMessage(executionArtifact(
			"exit-1", "BTC/USD", "sell", 0.1, 0.1, 90, "filled", "trade",
		))

		Convey("It should close and remove the durable stop only after the confirmed fill", func() {
			So(err, ShouldBeNil)

			stop := loadStop(tree, "BTC/USD")
			So(stop, ShouldNotBeNil)
			So(stop.Snapshot().Qty, ShouldEqual, 0)
			So(desk.snapshot().stoplosses["BTC/USD"], ShouldBeNil)
			So(desk.snapshot().orders["exit-1"].Symbol, ShouldEqual, "")
		})
	})
}

func TestDeskProtectsPartialFill(testingTB *testing.T) {
	Convey("Given a buy that fills in two confirmed chunks", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"type":      "market",
				"quantity":  0.2,
				"cl_ord_id": "open-1",
				"offset":    0.05,
			}.Marshal())

		So(desk.Update([]*datura.Artifact{action}), ShouldBeNil)
		So(awaitOrder(orders), ShouldNotBeNil)

		So(desk.observeExecutions(executionArtifact(
			"open-1", "BTC/USD", "buy", 0.2, 0.1, 100, "partially_filled", "trade",
		)), ShouldBeNil)

		first := loadStop(tree, "BTC/USD")
		So(first, ShouldNotBeNil)
		firstSnapshot := first.Snapshot()
		So(firstSnapshot.Qty, ShouldAlmostEqual, 0.1, 1e-9)
		So(firstSnapshot.Peak, ShouldAlmostEqual, 100, 1e-9)
		So(firstSnapshot.Stop, ShouldAlmostEqual, 95, 1e-9)

		So(desk.observeExecutions(executionArtifact(
			"open-1", "BTC/USD", "buy", 0.2, 0.2, 110, "filled", "trade",
		)), ShouldBeNil)

		Convey("It should expand the same stop without loosening it", func() {
			stop := loadStop(tree, "BTC/USD")

			So(stop, ShouldNotBeNil)
			snapshot := stop.Snapshot()
			So(snapshot.Qty, ShouldAlmostEqual, 0.2, 1e-9)
			So(snapshot.Peak, ShouldAlmostEqual, 110, 1e-9)
			So(snapshot.Stop, ShouldAlmostEqual, 104.5, 1e-9)
		})
	})
}

func seedTicker(desk *Desk, symbol string, last float64) {
	seedTickerQuote(desk, symbol, last, last, last)
}

func seedTickerQuote(desk *Desk, symbol string, last, bid, ask float64) {
	frame := fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":%g,"bid":%g,"ask":%g}]}`,
		symbol, last, bid, ask,
	)

	artifact := datura.Acquire("websocket", datura.APPJSON).
		WithDestination("desk").
		WithRole("ticker").
		WithScope("update").
		WithPayload([]byte(frame))

	if err := desk.onMessage(artifact); err != nil {
		panic(err)
	}
}

func seedExecution(desk *Desk, clOrdID, symbol, side string, qty, price float64) {
	artifact := executionArtifact(clOrdID, symbol, side, qty, qty, price, "filled", "trade")

	if err := desk.onMessage(artifact); err != nil {
		panic(err)
	}
}

func executionArtifact(
	clOrdID, symbol, side string,
	orderQty, cumQty, price float64,
	orderStatus, execType string,
) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"executions":{"exec-1":{"cl_ord_id":%q,"symbol":%q,"side":%q,"order_qty":%g,"cum_qty":%g,"last_price":%g,"avg_price":%g,"order_status":%q,"exec_type":%q}}}`,
		clOrdID, symbol, side, orderQty, cumQty, price, price, orderStatus, execType,
	)

	return datura.Acquire("kraken:private", datura.APPJSON).
		WithDestination("ui").
		WithRole("executions").
		WithScope("update").
		WithPayload([]byte(payload))
}

func storeStop(desk *Desk, stoploss *Stoploss) {
	if err := desk.putStop(stoploss); err != nil {
		panic(err)
	}
}

func loadStop(tree *dmt.Tree, symbol string) *Stoploss {
	for artifact := range tree.Seek([]byte("stoploss/" + symbol)) {
		stop := StoplossFromArtifact(artifact)
		artifact.Release()

		return stop
	}

	return nil
}

func captureOrders(pool *qpool.Q[any]) chan *datura.Artifact {
	received := make(chan *datura.Artifact, 8)

	pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
		received <- artifact

		return nil
	})

	return received
}

func awaitOrder(received chan *datura.Artifact) *datura.Artifact {
	select {
	case order := <-received:
		return order
	case <-time.After(time.Second):
		return nil
	}
}
