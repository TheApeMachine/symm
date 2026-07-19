package websocket

import (
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestPeekBookIndexedMissDoesNotScanThrash proves an indexed Live without a book
yet returns false without deleting the index or walking every connection — the
regression that collapsed tick rate under touchReady on a large L3 universe.
*/
func TestPeekBookIndexedMissDoesNotScanThrash(t *testing.T) {
	Convey("Given many Level3 transports with one indexed symbol and no book", t, func() {
		registry := NewLevel3Registry()
		owner := &Live{books: spot.NewBookManager(), symbols: []string{"VANRY/USD"}}
		registry.Attach("owner", owner)

		for index := range 64 {
			other := &Live{books: spot.NewBookManager(), symbols: []string{"OTHER/USD"}}
			registry.Attach(fmt.Sprintf("other-%d", index), other)
		}

		misses := 0

		for range 10_000 {
			if !registry.PeekBook("VANRY/USD", func(*book.Book) {}) {
				misses++
			}
		}

		Convey("Then every peek misses without clearing the index", func() {
			So(misses, ShouldEqual, 10_000)
			value, ok := registry.index.Load("VANRY/USD")
			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, owner)
		})

		Convey("When the book appears on the indexed Live", func() {
			managed := owner.books.CreateBook("VANRY/USD", 10)
			managed.Update(&book.UpdateOptions{
				Direction: book.Bid, ID: "bid",
				Price: decimal.NewFromFloat64(1), Quantity: decimal.NewFromFloat64(1),
				Timestamp: time.Unix(1, 0),
			})
			hit := registry.PeekBook("VANRY/USD", func(symbolBook *book.Book) {
				So(symbolBook, ShouldNotBeNil)
			})
			So(hit, ShouldBeTrue)
		})
	})
}
