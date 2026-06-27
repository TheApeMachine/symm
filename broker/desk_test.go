package broker

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
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

		Convey("The desk sells out and tombstones the stop", func() {
			order := awaitOrder(orders)

			So(order, ShouldNotBeNil)
			So(datura.Peek[string](order, "params", "side"), ShouldEqual, "sell")
			So(datura.Peek[float64](order, "params", "order_qty"), ShouldEqual, 0.1)

			stop := loadStop(tree, "BTC/USD")
			snapshot := stop.Snapshot()

			So(snapshot.Qty, ShouldEqual, 0)
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

		err := desk.observeExecutions(executionArtifact(
			"open-1", "BTC/USD", "buy", 0.1, 0.1, 101, "open", "accepted",
		))

		Convey("It should error and leave the position unarmed", func() {
			So(err, ShouldNotBeNil)
			So(loadStop(tree, "BTC/USD"), ShouldBeNil)
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
