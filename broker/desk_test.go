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
	Convey("Given a ticker in the tree and a buy action", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		seedTicker(tree, "BTC/USD", 100)

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)

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

		Convey("It sends a buy order and arms a trailing stop in the tree", func() {
			order := awaitOrder(orders)

			So(order, ShouldNotBeNil)
			So(datura.Peek[string](order, "params", "side"), ShouldEqual, "buy")
			So(datura.Peek[string](order, "params", "symbol"), ShouldEqual, "BTC/USD")
			So(datura.Peek[float64](order, "params", "order_qty"), ShouldEqual, 0.1)

			stop := loadStop(tree, "BTC/USD")

			So(stop, ShouldNotBeNil)
			So(stop.Qty, ShouldEqual, 0.1)
			So(stop.Peak, ShouldEqual, 100)
			So(stop.Stop, ShouldAlmostEqual, 95, 1e-9)
		})
	})
}

func TestDeskRatchetsStopUp(testingTB *testing.T) {
	Convey("Given an armed stop and a higher mark", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		storeStop(tree, NewStoploss("BTC/USD", 0.1, 100, 0.05))
		seedTicker(tree, "BTC/USD", 120)

		desk := NewDesk(ctx, pool, tree)

		So(desk.Update(nil), ShouldBeNil)

		Convey("The stop trails up and the position stays open", func() {
			stop := loadStop(tree, "BTC/USD")

			So(stop.Qty, ShouldEqual, 0.1)
			So(stop.Peak, ShouldEqual, 120)
			So(stop.Stop, ShouldAlmostEqual, 114, 1e-9)
		})
	})
}

func TestDeskExitsOnStopBreach(testingTB *testing.T) {
	Convey("Given an armed stop and a mark below it", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		storeStop(tree, NewStoploss("BTC/USD", 0.1, 100, 0.05))
		seedTicker(tree, "BTC/USD", 90)

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)

		So(desk.Update(nil), ShouldBeNil)

		Convey("The desk sells out and tombstones the stop", func() {
			order := awaitOrder(orders)

			So(order, ShouldNotBeNil)
			So(datura.Peek[string](order, "params", "side"), ShouldEqual, "sell")
			So(datura.Peek[float64](order, "params", "order_qty"), ShouldEqual, 0.1)

			stop := loadStop(tree, "BTC/USD")

			So(stop.Qty, ShouldEqual, 0)
		})
	})
}

func TestDeskRejectsEntryWithoutOffset(testingTB *testing.T) {
	Convey("Given a buy action with no stop offset", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		seedTicker(tree, "BTC/USD", 100)

		desk := NewDesk(ctx, pool, tree)

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

func seedTicker(tree *dmt.Tree, symbol string, last float64) {
	frame := fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":%g,"bid":%g,"ask":%g}]}`,
		symbol, last, last-0.5, last+0.5,
	)

	artifact := datura.Acquire("websocket", datura.APPJSON).
		WithRole("ticker").
		WithScope("update").
		WithPayload([]byte(frame))

	tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
}

func storeStop(tree *dmt.Tree, stoploss *Stoploss) {
	tree.InsertArtifact([]byte("stoploss/"+stoploss.Symbol), stoploss.Artifact())
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
