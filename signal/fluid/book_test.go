package fluid

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestBookMeasureWaitsForMarketState(t *testing.T) {
	Convey("Given a book without ticker-fed volume", t, func() {
		registry := NewSyncRegistry()
		book := NewBook(registry)
		eventAt := time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)

		Convey("When a snapshot arrives before market state is ready", func() {
			measurements, err := book.Measure(kraken.BookData{
				Symbol: "BTC/USD",
				Type:   "snapshot",
				Bids: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(99), Qty: 5},
				},
				Asks: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(101), Qty: 5},
				},
				Timestamp: eventAt,
			})

			Convey("Then it should wait instead of erroring", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeNil)
			})
		})
	})
}

func TestBookMeasureSkipsEmptyLevels(t *testing.T) {
	Convey("Given a book update with no levels on either side", t, func() {
		registry := NewSyncRegistry()
		book := NewBook(registry)
		eventAt := time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)

		Convey("When the frame is a checksum-only refresh or a thin market", func() {
			measurements, err := book.Measure(kraken.BookData{
				Symbol:    "BTC/USD",
				Type:      "update",
				Timestamp: eventAt,
			})

			Convey("Then it should skip without erroring", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeNil)
			})
		})

		Convey("When a real snapshot has already been recorded", func() {
			_, snapshotErr := book.Measure(kraken.BookData{
				Symbol: "BTC/USD",
				Type:   "snapshot",
				Bids: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(99), Qty: 5},
				},
				Asks: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(101), Qty: 5},
				},
				Timestamp: eventAt,
			})
			So(snapshotErr, ShouldBeNil)

			state := registry.loadSymbol("BTC/USD")
			bidsBefore := len(state.book.Bids)
			asksBefore := len(state.book.Asks)

			Convey("Then a subsequent empty-levels update should not erase the known book", func() {
				_, updateErr := book.Measure(kraken.BookData{
					Symbol:    "BTC/USD",
					Type:      "update",
					Timestamp: eventAt.Add(time.Second),
				})

				So(updateErr, ShouldBeNil)
				So(len(state.book.Bids), ShouldEqual, bidsBefore)
				So(len(state.book.Asks), ShouldEqual, asksBefore)
			})
		})
	})
}
