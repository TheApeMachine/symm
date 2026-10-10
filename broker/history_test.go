package broker_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
)

func snapshot(book *broker.Book, at time.Time, bidQty float64) {
	level := func(id string, price, qty float64) kraken.Level3Order {
		return kraken.Level3Order{
			OrderID: id, LimitPrice: decimal.NewFromFloat64(price),
			OrderQty: decimal.NewFromFloat64(qty), Timestamp: at, Event: "add",
		}
	}

	So(book.Update(&kraken.Level3{
		Channel: "level3",
		Type:    "snapshot",
		Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids:   []kraken.Level3Order{level("bid", 100, bidQty)},
			Asks:   []kraken.Level3Order{level("ask", 101, 1)},
		}},
	}), ShouldBeNil)
}

func bidAt(book *broker.Book, at time.Time) (float64, bool) {
	var qty float64
	var found bool

	book.BookAt("BTC/USD", at, func(view *broker.BookView) {
		qty, found = view.Bids[0].Quantity, true
	})

	return qty, found
}

func TestBookAt(t *testing.T) {
	Convey("Given a book whose live state has moved past the frames being read", t, func() {
		book := broker.NewBook(context.Background(), spot.NewNormalizer())
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		at := func(step int) time.Time { return origin.Add(time.Duration(step) * time.Second) }

		for step := 0; step <= 3; step++ {
			snapshot(book, at(step), float64(step+1))
		}

		Convey("A cold history misses a lookup it has not retained and learns its lag", func() {
			_, found := bidAt(book, at(0))
			So(found, ShouldBeFalse)
			So(book.HistoryMisses(), ShouldEqual, 1)
			So(book.HistoryHorizon(), ShouldEqual, 3*time.Second)

			Convey("After that, replay yields the as-of book, not the live one", func() {
				for step := 4; step <= 9; step++ {
					snapshot(book, at(step), float64(step+1))
				}

				live, found := bidAt(book, at(9))
				So(found, ShouldBeTrue)
				So(live, ShouldEqual, 10)

				for step := 6; step <= 9; step++ {
					qty, found := bidAt(book, at(step))
					So(found, ShouldBeTrue)
					So(qty, ShouldEqual, float64(step+1))
				}

				// Between versions, the earlier one is in effect.
				qty, found := bidAt(book, at(7).Add(500*time.Millisecond))
				So(found, ShouldBeTrue)
				So(qty, ShouldEqual, 8)
				So(book.HistoryMisses(), ShouldEqual, 1)
			})
		})

		Convey("A dropped stream leaves the book unknown after its last version", func() {
			book.Stale([]string{"BTC/USD"})

			qty, found := bidAt(book, at(3))
			So(found, ShouldBeTrue)
			So(qty, ShouldEqual, 4)

			_, found = bidAt(book, at(4))
			So(found, ShouldBeFalse)

			Convey("until a fresh snapshot restores it", func() {
				snapshot(book, at(5), 6)
				qty, found := bidAt(book, at(5))
				So(found, ShouldBeTrue)
				So(qty, ShouldEqual, 6)
			})
		})

		Convey("A symbol without history reads nothing", func() {
			read := false
			book.BookAt("ETH/USD", at(3), func(*broker.BookView) { read = true })
			So(read, ShouldBeFalse)
		})
	})
}
