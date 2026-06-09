package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookUpdateFoldSnapshot(t *testing.T) {
	Convey("Given an empty maintained book", t, func() {
		book := BookUpdate{}

		Convey("When a snapshot arrives", func() {
			book.Fold(BookUpdate{
				Type: "snapshot",
				Bids: []BookLevel{{Price: 99, Qty: 10}},
				Asks: []BookLevel{{Price: 101, Qty: 6}},
			}, 10)

			Convey("It should replace both sides", func() {
				So(len(book.Bids), ShouldEqual, 1)
				So(len(book.Asks), ShouldEqual, 1)
				So(book.Bids[0].Price, ShouldEqual, 99)
				So(book.Asks[0].Price, ShouldEqual, 101)
			})
		})
	})
}

func TestBookUpdateFoldUpdate(t *testing.T) {
	Convey("Given a maintained book", t, func() {
		book := BookUpdate{
			Bids: []BookLevel{{Price: 99, Qty: 10}},
			Asks: []BookLevel{{Price: 101, Qty: 6}},
		}

		Convey("When a level is removed", func() {
			book.Fold(BookUpdate{
				Bids: []BookLevel{{Price: 99, Qty: 0}},
			}, 10)

			Convey("It should drop that price", func() {
				So(len(book.Bids), ShouldEqual, 0)
				So(len(book.Asks), ShouldEqual, 1)
			})
		})
	})
}

func TestBookUpdateComputedChecksum(t *testing.T) {
	Convey("Given a two-sided book", t, func() {
		book := BookUpdate{
			Asks: []BookLevel{{Price: 45285.2, Qty: 0.001}},
			Bids: []BookLevel{{Price: 45284.9, Qty: 0.5}},
		}

		Convey("It should produce a stable checksum", func() {
			first := book.ComputedChecksum()
			second := book.ComputedChecksum()

			So(first, ShouldEqual, second)
			So(first, ShouldBeGreaterThan, 0)
		})
	})
}
