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

func TestOrdersRestore(t *testing.T) {
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
