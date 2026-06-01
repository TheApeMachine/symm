package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookFold(t *testing.T) {
	Convey("Given a book fed an update frame", t, func() {
		book := Book{}
		bids := []BookLevel{{Price: 99, Qty: 8}}
		asks := []BookLevel{{Price: 101, Qty: 4}}
		delta := Book{Bids: bids, Asks: asks}
		delta.SetEnvelopeType("update")
		book.Fold(delta, 10)

		Convey("It should merge the delta levels", func() {
			So(len(book.Bids), ShouldEqual, 1)
			So(len(book.Asks), ShouldEqual, 1)
			So(book.Bids[0].Price, ShouldEqual, 99)
		})
	})

	Convey("Given a book fed a checksum-valid snapshot", t, func() {
		book := Book{}
		bids := []BookLevel{{Price: 99, Qty: 8}}
		asks := []BookLevel{{Price: 101, Qty: 4}}
		snapshot := Book{
			Symbol: "ETH/EUR",
			Bids:   bids,
			Asks:   asks,
		}
		snapshot.SetEnvelopeType(BookSnapshot)
		book.Fold(snapshot, 10)

		Convey("It should retain the snapshot levels", func() {
			So(book.Bids[0].Price, ShouldEqual, 99)
			So(book.Asks[0].Price, ShouldEqual, 101)
		})

		Convey("It should match the exchange checksum", func() {
			frame := Book{Bids: book.Bids, Asks: book.Asks}

			So(frame.ComputedChecksum(), ShouldEqual, snapshot.ComputedChecksum())
		})
	})
}

func BenchmarkBookFold(b *testing.B) {
	bids := []BookLevel{{Price: 99, Qty: 8}}
	asks := []BookLevel{{Price: 101, Qty: 4}}
	update := Book{Symbol: "ETH/EUR", Bids: bids, Asks: asks}
	update.SetEnvelopeType(BookSnapshot)

	b.ReportAllocs()

	for b.Loop() {
		book := Book{}
		book.Fold(update, 10)
	}
}
