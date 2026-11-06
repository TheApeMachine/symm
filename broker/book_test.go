package broker

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
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

	Convey("Delete event removes the resting order even when qty stays positive", t, func() {
		manager := NewBookManager()
		bid := decimal.RequireFromString("100")
		qty := decimal.RequireFromString("1.5")

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					OrderID:    "b1",
					LimitPrice: &bid,
					OrderQty:   &qty,
					Timestamp:  time.Unix(1, 0),
				}},
			}},
		})

		So(manager.Get("BTC/USD").BestBid(), ShouldNotBeNil)

		manager.Update(&market.Level3{
			Type: "update",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					Event:      "delete",
					OrderID:    "b1",
					LimitPrice: &bid,
					OrderQty:   &qty,
					Timestamp:  time.Unix(2, 0),
				}},
			}},
		})

		So(manager.Get("BTC/USD").BestBid(), ShouldBeNil)
	})

	Convey("Snapshot replaces prior book state", t, func() {
		manager := NewBookManager()
		oldBid := decimal.RequireFromString("99")
		newBid := decimal.RequireFromString("100")
		qty := decimal.RequireFromString("1")

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					OrderID:    "ghost",
					LimitPrice: &oldBid,
					OrderQty:   &qty,
					Timestamp:  time.Unix(1, 0),
				}},
			}},
		})

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					OrderID:    "live",
					LimitPrice: &newBid,
					OrderQty:   &qty,
					Timestamp:  time.Unix(2, 0),
				}},
			}},
		})

		book := manager.Get("BTC/USD")
		So(book.BestBid(), ShouldNotBeNil)
		So(book.BestBid().Price.String(), ShouldEqual, "100")
		So(len(book.Bids.Levels), ShouldEqual, 1)
		So(book.MaxDepth, ShouldEqual, system.Cfg.Market.Level3.Depth)
	})

	Convey("A crossed book is marked stale and notifies recovery", t, func() {
		manager := NewBookManager()
		notified := []string{}
		manager.OnInvalid(func(symbols []string) {
			notified = append(notified, symbols...)
		})

		bid := decimal.RequireFromString("101")
		ask := decimal.RequireFromString("100")
		qty := decimal.RequireFromString("1")

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bid, OrderQty: &qty,
				}},
				Asks: []*market.Level3Order{{
					OrderID: "a1", LimitPrice: &ask, OrderQty: &qty,
				}},
			}},
		})

		So(manager.Get("BTC/USD"), ShouldBeNil)
		So(manager.StaleSymbol("BTC/USD"), ShouldBeTrue)
		So(notified, ShouldResemble, []string{"BTC/USD"})
	})
}
