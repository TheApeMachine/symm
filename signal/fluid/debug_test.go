package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestFluidSymbolPartialBookUpdatePreservesRestingSide(t *testing.T) {
	Convey("Given a live book and a one-sided update", t, withFluidGrid(nil, func() {
		state, err := NewFluidSymbol("ETH/EUR")
		So(err, ShouldBeNil)

		fixture := symbolBookFixture{symbol: "ETH/EUR"}
		start := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

		So(state.FeedBook(fixture.snapshot(99.99, 5, 100.01, 5), start), ShouldBeNil)

		update := kraken.BookData{
			Symbol: "ETH/EUR",
			Type:   "update",
			Bids: []kraken.BookLevel{
				testBookLevel("99.99", 6),
			},
		}

		So(state.FeedBook(update, start.Add(100*time.Millisecond)), ShouldBeNil)

		Convey("It should keep the last ask side instead of treating it as deleted", func() {
			So(len(state.book.Bids), ShouldEqual, 2)
			So(len(state.book.Asks), ShouldEqual, 2)
			So(state.book.Asks[0].Price.Float64(), ShouldAlmostEqual, 100.01, 1e-12)
			So(state.book.Asks[0].Qty, ShouldAlmostEqual, 5, 1e-12)
		})
	}))
}

func TestFluidSymbolUpdateBeforeSnapshotWaits(t *testing.T) {
	Convey("Given a fluid symbol without a book snapshot", t, func() {
		state, err := NewFluidSymbol("ETH/EUR")
		So(err, ShouldBeNil)

		Convey("When a book update arrives before the snapshot", func() {
			err := state.FeedBook(kraken.BookData{
				Symbol: "ETH/EUR",
				Type:   "update",
				Bids:   []kraken.BookLevel{testBookLevel("99.99", 5)},
			}, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))

			Convey("It should wait for the first snapshot", func() {
				So(err, ShouldBeNil)
				So(state.HasBook(), ShouldBeFalse)
			})
		})
	})
}
