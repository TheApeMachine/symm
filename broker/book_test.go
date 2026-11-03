package broker

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestBookManagerUpdate(t *testing.T) {
	Convey("Level3 bids and asks land on the matching symbol book", t, func() {
		manager := NewBookManager()
		bid := decimal.RequireFromString("100")
		bidQty := decimal.RequireFromString("1.5")
		ask := decimal.RequireFromString("101")
		askQty := decimal.RequireFromString("2")

		manager.Update(&market.Level3{
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					OrderID:    "b1",
					LimitPrice: &bid,
					OrderQty:   &bidQty,
					Timestamp:  time.Unix(1, 0),
				}},
				Asks: []*market.Level3Order{{
					OrderID:    "a1",
					LimitPrice: &ask,
					OrderQty:   &askQty,
					Timestamp:  time.Unix(1, 0),
				}},
			}},
		})

		book := manager.Get("BTC/USD")
		So(book, ShouldNotBeNil)
		So(book.BestBid(), ShouldNotBeNil)
		So(book.BestBid().Price.String(), ShouldEqual, "100")
		So(book.BestAsk(), ShouldNotBeNil)
		So(book.BestAsk().Price.String(), ShouldEqual, "101")
	})

	Convey("Nil frame is a no-op", t, func() {
		manager := NewBookManager()
		So(func() { manager.Update(nil) }, ShouldNotPanic)
		So(manager.Get("BTC/USD"), ShouldBeNil)
	})
}
