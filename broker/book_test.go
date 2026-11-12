package broker

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
)

func TestBookManager(t *testing.T) {
	Convey("Setup", t, func() {
	Convey("When Level3 bids and asks land on the matching symbol book", func() {
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

	Convey("When the frame is nil Update is a no-op", func() {
		manager := NewBookManager()
		So(func() { manager.Update(nil) }, ShouldNotPanic)
		So(manager.Get("BTC/USD"), ShouldBeNil)
	})

	Convey("When a delete event arrives the resting order is removed", func() {
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

	Convey("When a snapshot arrives it replaces prior book state", func() {
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

	Convey("When the book is crossed it is marked stale and recovery is notified", func() {
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

	Convey("When BTC is updated concurrently ETH stays readable", func() {
		manager := NewBookManager()
		bid := decimal.RequireFromString("100")
		ask := decimal.RequireFromString("101")
		qty := decimal.RequireFromString("1")

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "ETH/USD",
				Bids: []*market.Level3Order{{
					OrderID: "e1", LimitPrice: &bid, OrderQty: &qty,
				}},
				Asks: []*market.Level3Order{{
					OrderID: "e2", LimitPrice: &ask, OrderQty: &qty,
				}},
			}},
		})

		btcBid := decimal.RequireFromString("200")
		btcAsk := decimal.RequireFromString("201")

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()

			manager.Update(&market.Level3{
				Type: "snapshot",
				Data: []*market.Level3Data{{
					Symbol: "BTC/USD",
					Bids: []*market.Level3Order{{
						OrderID: "b1", LimitPrice: &btcBid, OrderQty: &qty,
					}},
					Asks: []*market.Level3Order{{
						OrderID: "b2", LimitPrice: &btcAsk, OrderQty: &qty,
					}},
				}},
			})
		}()

		var ethBid *decimal.Decimal

		manager.Book("ETH/USD", func(book *market.Book) {
			if best := book.BestBid(); best != nil {
				ethBid = best.Price
			}
		})

		wg.Wait()

		So(ethBid, ShouldNotBeNil)
		So(ethBid.String(), ShouldEqual, "100")
		So(manager.Get("BTC/USD"), ShouldNotBeNil)
	})

	Convey("When symbols are expected duplicates and empties are skipped", func() {
		manager := NewBookManager()
		manager.Expect([]string{"BTC/USD", "", "BTC/USD", "ETH/USD"})

		So(manager.Pending(), ShouldResemble, []string{"BTC/USD", "ETH/USD"})
		So(manager.pendingN.Load(), ShouldEqual, int64(2))
	})

	Convey("When snapshot is still expected BlockReason and Wait report pending", func() {
		manager := NewBookManager()
		manager.Expect([]string{"BTC/USD"})

		So(manager.BlockReason("BTC/USD"), ShouldEqual, "level3 book still pending snapshot")

		err := manager.Wait(20 * time.Millisecond)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "not seeded")
		So(err.Error(), ShouldContainSubstring, "BTC/USD")
	})

	Convey("When the snapshot lands Wait returns nil", func() {
		manager := NewBookManager()
		manager.Expect([]string{"ETH/USD"})

		waited := make(chan error, 1)
		go func() {
			waited <- manager.Wait(200 * time.Millisecond)
		}()

		bid := decimal.RequireFromString("50")
		qty := decimal.RequireFromString("1")
		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "ETH/USD",
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bid, OrderQty: &qty,
				}},
			}},
		})

		select {
		case err := <-waited:
			So(err, ShouldBeNil)
		case <-time.After(2 * time.Second):
			t.Fatal("Wait did not unblock after snapshot")
		}
		So(manager.Pending(), ShouldBeEmpty)
		So(manager.Get("ETH/USD"), ShouldNotBeNil)
	})

	Convey("When an update amends resting qty the book reflects the new size", func() {
		manager := NewBookManager()
		bid := decimal.RequireFromString("100")
		q1 := decimal.RequireFromString("1")
		q3 := decimal.RequireFromString("3")

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bid, OrderQty: &q1,
				}},
			}},
		})

		manager.Update(&market.Level3{
			Type: "update",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					Event: "update", OrderID: "b1", LimitPrice: &bid, OrderQty: &q3,
				}},
			}},
		})

		book := manager.Get("BTC/USD")
		So(book.BestBid().Quantity.String(), ShouldEqual, "3")
	})

	Convey("When pending clears BlockReason is empty", func() {
		manager := NewBookManager()
		manager.Expect([]string{"BTC/USD"})
		bid := decimal.RequireFromString("1")
		qty := decimal.RequireFromString("1")

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bid, OrderQty: &qty,
				}},
			}},
		})

		So(manager.BlockReason("BTC/USD"), ShouldEqual, "")
		So(manager.StaleSymbol("BTC/USD"), ShouldBeFalse)
	})

	Convey("When Stale is called reads fail until snapshot returns", func() {
		manager := NewBookManager()
		bid := decimal.RequireFromString("100")
		qty := decimal.RequireFromString("1")

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "SOL/USD",
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bid, OrderQty: &qty,
				}},
			}},
		})

		manager.Stale([]string{"SOL/USD"})
		So(manager.StaleSymbol("SOL/USD"), ShouldBeTrue)
		So(manager.Get("SOL/USD"), ShouldBeNil)
	})

	Convey("When All yields every live book", func() {
		manager := NewBookManager()
		bid := decimal.RequireFromString("1")
		qty := decimal.RequireFromString("1")

		for _, sym := range []string{"AAA/USD", "BBB/USD"} {
			manager.Update(&market.Level3{
				Type: "snapshot",
				Data: []*market.Level3Data{{
					Symbol: sym,
					Bids: []*market.Level3Order{{
						OrderID: "b", LimitPrice: &bid, OrderQty: &qty,
					}},
				}},
			})
		}

		seen := 0
		for book := range manager.All() {
			if book != nil {
				seen++
			}
		}

		So(seen, ShouldEqual, 2)
	})

	Convey("When venue checksum diverges the lane is marked stale", func() {
		manager := NewBookManager()
		var stale []string
		manager.OnInvalid(func(symbols []string) { stale = append(stale, symbols...) })

		bid := decimal.RequireFromString("100")
		bidQty := decimal.RequireFromString("1.5")
		ask := decimal.RequireFromString("101")
		askQty := decimal.RequireFromString("2")

		manager.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol:   "CRC/USD",
				Checksum: 0,
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bid, OrderQty: &bidQty,
				}},
				Asks: []*market.Level3Order{{
					OrderID: "a1", LimitPrice: &ask, OrderQty: &askQty,
				}},
			}},
		})

		book := manager.Get("CRC/USD")
		So(book, ShouldNotBeNil)

		match, err := strconv.Atoi(book.L3Checksum("0").LocalChecksum)
		So(err, ShouldBeNil)

		manager.Update(&market.Level3{
			Type: "update",
			Data: []*market.Level3Data{{
				Symbol:   "CRC/USD",
				Checksum: match,
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bid, OrderQty: &bidQty,
				}},
				Asks: []*market.Level3Order{{
					OrderID: "a1", LimitPrice: &ask, OrderQty: &askQty,
				}},
			}},
		})

		So(manager.StaleSymbol("CRC/USD"), ShouldBeFalse)

		manager.Update(&market.Level3{
			Type: "update",
			Data: []*market.Level3Data{{
				Symbol:   "CRC/USD",
				Checksum: match + 1,
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bid, OrderQty: &bidQty,
				}},
				Asks: []*market.Level3Order{{
					OrderID: "a1", LimitPrice: &ask, OrderQty: &askQty,
				}},
			}},
		})

		So(manager.StaleSymbol("CRC/USD"), ShouldBeTrue)
		So(stale, ShouldContain, "CRC/USD")
	})
	})
}
