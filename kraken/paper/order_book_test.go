package paper

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestOrdersStoreAndTake(t *testing.T) {
	Convey("Given open orders", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		defer ws.Close()

		orders := ws.sockets["orders"].(*Orders)
		order := &openOrder{
			orderID:   "order-1",
			clOrdID:   "cl-1",
			symbol:    "BTC/EUR",
			side:      trading.Buy,
			orderType: trading.Limit,
			orderQty:  0.5,
		}

		orders.storeOrder(order)

		Convey("It should find orders by id and cl_ord_id", func() {
			byID, ok := orders.orderByID("order-1")
			So(ok, ShouldBeTrue)
			So(byID.clOrdID, ShouldEqual, "cl-1")

			byCl, ok := orders.orderByClOrdID("cl-1")
			So(ok, ShouldBeTrue)
			So(byCl.orderID, ShouldEqual, "order-1")
		})

		Convey("It should remove orders on take", func() {
			taken, ok := orders.takeOrder("order-1")

			So(ok, ShouldBeTrue)
			So(taken.clOrdID, ShouldEqual, "cl-1")
			So(orders.openOrderIDs(), ShouldBeEmpty)
		})
	})
}

func TestOrdersAmendStored(t *testing.T) {
	Convey("Given a resting order", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		defer ws.Close()

		orders := ws.sockets["orders"].(*Orders)
		order := &openOrder{orderID: "o1", orderQty: 1, limitPrice: 100}
		orders.storeOrder(order)
		orders.amendStored(order, 2, 101)

		stored, ok := orders.orderByID("o1")

		Convey("It should update quantity and limit price", func() {
			So(ok, ShouldBeTrue)
			So(stored.orderQty, ShouldEqual, 2)
			So(stored.limitPrice, ShouldEqual, 101)
		})
	})
}
