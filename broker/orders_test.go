package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

const ordersTwoOpen = `{
	"error": [],
	"result": {
		"o1": {
			"cl_ord_id": "enter-bera",
			"status": "closed",
			"opentm": 1700000000.0,
			"vol": "10.0",
			"vol_exec": "10.0",
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
			"opentm": 1700000100.0,
			"vol": "500.0",
			"vol_exec": "500.0",
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
			"opentm": 1700000000.0,
			"vol": "1.0",
			"vol_exec": "1.0",
			"descr": {"pair": "XBTUSD", "type": "buy", "ordertype": "market", "price": "0"}
		},
		"o2": {
			"cl_ord_id": "exit-1",
			"status": "closed",
			"opentm": 1700000100.0,
			"vol": "1.0",
			"vol_exec": "1.0",
			"descr": {"pair": "XBTUSD", "type": "sell", "ordertype": "market", "price": "0"}
		},
		"o3": {
			"cl_ord_id": "enter-2",
			"status": "closed",
			"opentm": 1700000200.0,
			"vol": "1.0",
			"vol_exec": "1.0",
			"descr": {"pair": "XBTUSD", "type": "buy", "ordertype": "market", "price": "0"}
		},
		"o4": {
			"cl_ord_id": "exit-2",
			"status": "closed",
			"opentm": 1700000300.0,
			"vol": "1.0",
			"vol_exec": "1.0",
			"descr": {"pair": "XBTUSD", "type": "sell", "ordertype": "market", "price": "0"}
		}
	}
}`

func TestOrdersFetch(t *testing.T) {
	Convey("Setup", t, func() {
		if system.Cfg == nil || system.Cfg.WebSocket == nil {
			system.Cfg = system.NewConfig()
		}

		Convey("When Fetch gets venue rows for fill ordertxids", func() {
			transport := &historyClient{orders: []byte(ordersTwoOpen)}
			book := NewOrders(transport).Fetch([]string{"o1", "o2"})

			Convey("Each txid carries side and cl_ord_id", func() {
				So(len(book), ShouldEqual, 2)
				So(book["o1"].ClOrdID, ShouldEqual, "enter-bera")
				So(book["o1"].Descr.Type, ShouldEqual, "buy")
				So(book["o2"].ClOrdID, ShouldEqual, "enter-pepe")
			})
		})
	})
}

func TestOrdersRestoreViaLedger(t *testing.T) {
	Convey("Setup", t, func() {
		if system.Cfg == nil || system.Cfg.WebSocket == nil {
			system.Cfg = system.NewConfig()
		}

		cache := NewCache(
			t.Context(),
			"real",
			&historyClient{trades: []byte(historyTwoClosed), orders: []byte(ordersTwoClosed)},
			nil, nil, nil,
		)
		closed := cache.Closed()

		Convey("When closed cycles bind orders then fills", func() {
			So(len(closed), ShouldEqual, 2)

			byEntry := map[string]*Position{}

			for _, position := range closed {
				byEntry[position.EntryVenueOrderID] = position
			}

			first := byEntry["o1"]
			second := byEntry["o3"]

			Convey("Entry and exit orders stamp from QueryOrders", func() {
				So(first, ShouldNotBeNil)
				So(first.ExitVenueOrderID, ShouldEqual, "o2")
				So(first.EntryOrder, ShouldNotBeNil)
				So(first.EntryOrder.ClOrdId, ShouldEqual, "enter-1")
				So(first.ExitOrder, ShouldNotBeNil)
				So(first.ExitOrder.ClOrdId, ShouldEqual, "exit-1")

				So(second, ShouldNotBeNil)
				So(second.ExitVenueOrderID, ShouldEqual, "o4")
				So(second.EntryOrder, ShouldNotBeNil)
				So(second.ExitOrder, ShouldNotBeNil)
			})
		})

		Convey("When open buys restore through QueryOrders", func() {
			openCache := NewCache(
				t.Context(),
				"real",
				&historyClient{
					trades:   []byte(historyTwoOpen),
					orders:   []byte(ordersTwoOpen),
					balances: []byte(walletTwoOpen),
				},
				nil, nil, nil,
			)
			open := openCache.Open()

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
	})
}
