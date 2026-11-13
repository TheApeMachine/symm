package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func level3Snapshot(symbol string, bidPx, askPx string) *market.Level3 {
	bid := decimal.RequireFromString(bidPx)
	bidQty := decimal.RequireFromString("10")
	ask := decimal.RequireFromString(askPx)
	askQty := decimal.RequireFromString("10")

	return &market.Level3{
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				OrderID: "b1", LimitPrice: &bid, OrderQty: &bidQty,
				Timestamp: time.Unix(1, 0),
			}},
			Asks: []*market.Level3Order{{
				OrderID: "a1", LimitPrice: &ask, OrderQty: &askQty,
				Timestamp: time.Unix(1, 0),
			}},
		}},
	}
}

func withBookManager(f func(*BookManager)) func() {
	return func() {
		normalizer := spot.NewNormalizer()
		seedPair(normalizer, testSymbol)
		f(NewBookManager(normalizer))
	}
}

func TestNewBookManager(t *testing.T) {
	Convey("Setup", t, withBookManager(func(mgr *BookManager) {
		Convey("When no symbol was seeded yet", func() {
			Convey("Get returns nil", func() {
				So(mgr.Get(testSymbol), ShouldBeNil)
				So(mgr.BlockReason(testSymbol), ShouldEqual, "level3 book not seeded")
			})
		})
	}))
}

func TestBookUpdate(t *testing.T) {
	Convey("Setup", t, withBookManager(func(mgr *BookManager) {
		Convey("When a snapshot lands", func() {
			mgr.Update(level3Snapshot(testSymbol, "100", "101"))

			Convey("Best bid and ask match the frame", func() {
				book := mgr.Get(testSymbol)
				So(book, ShouldNotBeNil)
				So(book.BestBid().Price.String(), ShouldEqual, "100")
				So(book.BestAsk().Price.String(), ShouldEqual, "101")
				So(mgr.BlockReason(testSymbol), ShouldEqual, "")
			})
		})

		Convey("When the book is crossed", func() {
			var invalid sync.WaitGroup
			invalid.Add(1)
			var symbols []string

			mgr.OnInvalid(func(ss []string) {
				symbols = ss
				invalid.Done()
			})
			mgr.Update(level3Snapshot(testSymbol, "101", "100"))

			invalid.Wait()

			Convey("OnInvalid fires and the lane is stale", func() {
				So(symbols, ShouldResemble, []string{testSymbol})
				So(mgr.Get(testSymbol), ShouldBeNil)
				So(mgr.StaleSymbol(testSymbol), ShouldBeTrue)
				So(mgr.MarkPaused(testSymbol), ShouldBeTrue)
			})
		})
	}))
}

func TestExpect(t *testing.T) {
	Convey("Setup", t, withBookManager(func(mgr *BookManager) {
		Convey("When symbols are expected before snapshots", func() {
			mgr.Expect([]string{testSymbol})

			Convey("Pending lists the symbol", func() {
				So(mgr.Pending(), ShouldResemble, []string{testSymbol})
				So(mgr.MarkPaused(testSymbol), ShouldBeTrue)
				So(mgr.BlockReason(testSymbol), ShouldEqual, "level3 book still pending snapshot")
			})
		})
	}))
}

func TestPending(t *testing.T) {
	Convey("Setup", t, withBookManager(func(mgr *BookManager) {
		mgr.Expect([]string{testSymbol, "ETH/USD"})

		Convey("When one symbol seeds", func() {
			mgr.Update(level3Snapshot(testSymbol, "100", "101"))

			Convey("Pending keeps the unseeded symbol", func() {
				So(mgr.Pending(), ShouldResemble, []string{"ETH/USD"})
			})
		})
	}))
}

func TestWait(t *testing.T) {
	Convey("Setup", t, withBookManager(func(mgr *BookManager) {
		Convey("When every expected symbol seeds", func() {
			mgr.Expect([]string{testSymbol})
			go func() {
				time.Sleep(10 * time.Millisecond)
				mgr.Update(level3Snapshot(testSymbol, "100", "101"))
			}()

			err := mgr.Wait(200 * time.Millisecond)

			Convey("Wait returns nil", func() {
				So(err, ShouldBeNil)
				So(mgr.Pending(), ShouldBeEmpty)
			})
		})

		Convey("When snapshots never arrive", func() {
			mgr.Expect([]string{testSymbol})

			err := mgr.Wait(50 * time.Millisecond)

			Convey("Wait times out with book detail", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "book: 1 level3 books not seeded")
				So(err.Error(), ShouldContainSubstring, testSymbol)
			})
		})
	}))
}

func TestOnInvalid(t *testing.T) {
	Convey("Setup", t, withBookManager(func(mgr *BookManager) {
		called := false
		mgr.OnInvalid(func([]string) { called = true })
		mgr.Update(level3Snapshot(testSymbol, "100", "101"))

		Convey("When a clean snapshot follows", func() {
			mgr.Update(level3Snapshot(testSymbol, "99", "100"))

			Convey("OnInvalid is not called again", func() {
				So(called, ShouldBeFalse)
				So(mgr.StaleSymbol(testSymbol), ShouldBeFalse)
			})
		})
	}))
}

func TestMarkPaused(t *testing.T) {
	Convey("Setup", t, withBookManager(func(mgr *BookManager) {
		Convey("When the book is live", func() {
			mgr.Update(level3Snapshot(testSymbol, "100", "101"))

			Convey("MarkPaused is false", func() {
				So(mgr.MarkPaused(testSymbol), ShouldBeFalse)
			})
		})
	}))
}

func TestStale(t *testing.T) {
	Convey("Setup", t, withBookManager(func(mgr *BookManager) {
		mgr.Update(level3Snapshot(testSymbol, "100", "101"))

		Convey("When Stale is called on a symbol", func() {
			mgr.Stale([]string{testSymbol})

			Convey("StaleSymbol is true and BlockReason cites stale", func() {
				So(mgr.StaleSymbol(testSymbol), ShouldBeTrue)
				So(mgr.Get(testSymbol), ShouldBeNil)
				So(mgr.BlockReason(testSymbol), ShouldEqual, "level3 book is stale")
			})
		})
	}))
}

func BenchmarkNewBookManager(b *testing.B) {
	normalizer := spot.NewNormalizer()
	seedPair(normalizer, testSymbol)

	b.ReportAllocs()

	for b.Loop() {
		_ = NewBookManager(normalizer)
	}
}

func BenchmarkBookUpdate(b *testing.B) {
	var mgr *BookManager
	withBookManager(func(m *BookManager) { mgr = m })()
	snap := level3Snapshot(testSymbol, "100", "101")

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mgr.Update(snap)
	}
}

func BenchmarkExpect(b *testing.B) {
	var mgr *BookManager
	withBookManager(func(m *BookManager) { mgr = m })()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mgr.Expect([]string{testSymbol})
	}
}

func BenchmarkPending(b *testing.B) {
	var mgr *BookManager
	withBookManager(func(m *BookManager) {
		mgr = m
		mgr.Expect([]string{testSymbol, "ETH/USD"})
		mgr.Update(level3Snapshot(testSymbol, "100", "101"))
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = mgr.Pending()
	}
}

func BenchmarkOnInvalid(b *testing.B) {
	var mgr *BookManager
	withBookManager(func(m *BookManager) {
		mgr = m
		mgr.Update(level3Snapshot(testSymbol, "100", "101"))
	})()
	snap := level3Snapshot(testSymbol, "99", "100")

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mgr.Update(snap)
	}
}

func BenchmarkMarkPaused(b *testing.B) {
	var mgr *BookManager
	withBookManager(func(m *BookManager) {
		mgr = m
		mgr.Update(level3Snapshot(testSymbol, "100", "101"))
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = mgr.MarkPaused(testSymbol)
	}
}

func BenchmarkStale(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		var mgr *BookManager
		withBookManager(func(m *BookManager) {
			mgr = m
			mgr.Update(level3Snapshot(testSymbol, "100", "101"))
		})()
		mgr.Stale([]string{testSymbol})
	}
}

func BenchmarkWait(b *testing.B) {
	var mgr *BookManager
	withBookManager(func(m *BookManager) {
		mgr = m
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = mgr.Wait(time.Hour)
	}
}
