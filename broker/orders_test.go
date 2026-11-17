package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

const ordersTwoOpen = `{
	"error": [],
	"result": {
		"o1": {
			"cl_ord_id": "enter-bera",
			"status": "closed",
			"vol": "10.0",
			"descr": {
				"pair": "BERAUSD",
				"type": "buy",
				"ordertype": "market",
				"price": "0"
			}
		},
		"o2": {
			"cl_ord_id": "enter-pepe",
			"status": "closed",
			"vol": "500.0",
			"descr": {
				"pair": "PEPEUSD",
				"type": "buy",
				"ordertype": "market",
				"price": "0"
			}
		}
	}
}`

const ordersTwoClosed = `{
	"error": [],
	"result": {
		"o1": {
			"cl_ord_id": "enter-1",
			"status": "closed",
			"vol": "1.0",
			"descr": {"pair": "XBTUSD", "type": "buy", "ordertype": "market", "price": "0"}
		},
		"o2": {
			"cl_ord_id": "exit-1",
			"status": "closed",
			"vol": "1.0",
			"descr": {"pair": "XBTUSD", "type": "sell", "ordertype": "market", "price": "0"}
		},
		"o3": {
			"cl_ord_id": "enter-2",
			"status": "closed",
			"vol": "1.0",
			"descr": {"pair": "XBTUSD", "type": "buy", "ordertype": "market", "price": "0"}
		},
		"o4": {
			"cl_ord_id": "exit-2",
			"status": "closed",
			"vol": "1.0",
			"descr": {"pair": "XBTUSD", "type": "sell", "ordertype": "market", "price": "0"}
		}
	}
}`

func TestOrdersRestore(t *testing.T) {
	Convey("Setup", t, func() {
		cache := NewCache(
			t.Context(),
			"real",
			&historyClient{trades: []byte(historyTwoClosed), orders: []byte(ordersTwoClosed)},
			nil, nil, nil,
		)
		closed := cache.Closed()

		Convey("When a cycle has an entry order and an exit order", func() {
			So(len(closed), ShouldEqual, 2)

			byEntry := map[string]*Position{}

			for _, position := range closed {
				byEntry[position.EntryVenueOrderID] = position
			}

			first := byEntry["o1"]
			second := byEntry["o3"]

			Convey("Both orders are bound on that holding", func() {
				So(first, ShouldNotBeNil)
				So(first.EntryOrder, ShouldNotBeNil)
				So(first.EntryOrder.ClOrdId, ShouldEqual, "enter-1")
				So(first.EntryOrder.Type, ShouldEqual, "buy")
				So(first.ExitOrder, ShouldNotBeNil)
				So(first.ExitOrder.ClOrdId, ShouldEqual, "exit-1")
				So(first.ExitOrder.Type, ShouldEqual, "sell")
				So(first.ExitVenueOrderID, ShouldEqual, "o2")

				So(second, ShouldNotBeNil)
				So(second.EntryOrder.ClOrdId, ShouldEqual, "enter-2")
				So(second.ExitOrder.ClOrdId, ShouldEqual, "exit-2")
				So(second.ExitVenueOrderID, ShouldEqual, "o4")
			})
		})
	})

	Convey("Setup", t, withHistory(historyTwoOpen, func(cache *Cache) {
		transport := &historyClient{
			trades: []byte(historyTwoOpen),
			orders: []byte(ordersTwoOpen),
		}

		Convey("When Orders.Restore runs after trades admit opens", func() {
			NewOrders(transport).Restore(cache.Open())
			open := cache.Open()

			Convey("Each open position carries EntryOrder and venue order id", func() {
				So(len(open), ShouldEqual, 2)

				bySymbol := map[string]*Position{}

				for _, position := range open {
					bySymbol[position.Symbol] = position
				}

				bera := bySymbol["BERAUSD"]
				So(bera, ShouldNotBeNil)
				So(bera.EntryVenueOrderID, ShouldEqual, "o1")
				So(bera.EntryOrder, ShouldNotBeNil)
				So(bera.EntryOrder.ClOrdId, ShouldEqual, "enter-bera")
				So(bera.EntryOrder.Type, ShouldEqual, "buy")
				So(bera.EntryOrder.Volume, ShouldEqual, "10")

				pepe := bySymbol["PEPEUSD"]
				So(pepe, ShouldNotBeNil)
				So(pepe.EntryVenueOrderID, ShouldEqual, "o2")
				So(pepe.EntryOrder, ShouldNotBeNil)
				So(pepe.EntryOrder.ClOrdId, ShouldEqual, "enter-pepe")
			})
		})
	}))
}
