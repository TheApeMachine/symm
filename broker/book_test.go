package broker_test

import (
	"context"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
)

func level3Frame(frameType, symbol string, bid, ask float64) *kraken.Level3 {
	now := time.Now().UTC()

	return &kraken.Level3{
		Channel: "level3",
		Type:    frameType,
		Data: []kraken.Level3Data{{
			Symbol:    symbol,
			Timestamp: now,
			Bids: []kraken.Level3Order{{
				OrderID: "b-" + frameType, LimitPrice: decimal.NewFromFloat64(bid),
				OrderQty: decimal.NewFromFloat64(1), Timestamp: now, Event: "add",
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "a-" + frameType, LimitPrice: decimal.NewFromFloat64(ask),
				OrderQty: decimal.NewFromFloat64(1), Timestamp: now, Event: "add",
			}},
		}},
	}
}

func TestBook_StaleUntilSnapshot(t *testing.T) {
	Convey("Given a seeded Level3 book", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		book := broker.NewBook(ctx, spot.NewNormalizer())
		So(book.Update(level3Frame("snapshot", "BTC/USD", 100, 101)), ShouldBeNil)

		served := func() bool {
			seen := false
			book.Book("BTC/USD", func(*spotbook.Book) { seen = true })
			return seen
		}

		So(served(), ShouldBeTrue)

		Convey("When its Level3 socket drops", func() {
			book.Stale([]string{"BTC/USD"})

			Convey("Readers get nothing and deltas are dropped", func() {
				So(served(), ShouldBeFalse)
				So(book.Update(level3Frame("update", "BTC/USD", 99, 102)), ShouldBeNil)
				So(served(), ShouldBeFalse)
			})

			Convey("A fresh snapshot restores the symbol", func() {
				So(book.Update(level3Frame("snapshot", "BTC/USD", 100, 101)), ShouldBeNil)
				So(served(), ShouldBeTrue)
			})
		})
	})
}
