package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluidSymbolPartialBookUpdatePreservesRestingSide(t *testing.T) {
	Convey("Given a live book and a one-sided update", t, func() {
		setFluidGridConfig()

		state, err := NewFluidSymbol("ETH/EUR")
		So(err, ShouldBeNil)

		fixture := symbolBookFixture{symbol: "ETH/EUR"}
		start := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

		So(state.FeedBook(fixture.snapshot(99.99, 5, 100.01, 5), start), ShouldBeNil)

		update := BookUpdate{
			Symbol: "ETH/EUR",
			Type:   "update",
			Bids: []BookLevel{
				{Price: 99.99, Qty: 6},
				{Price: 99.98, Qty: 5},
			},
		}

		So(state.FeedBook(update, start.Add(100*time.Millisecond)), ShouldBeNil)

		Convey("It should keep the last ask side instead of treating it as deleted", func() {
			So(len(state.book.Bids), ShouldEqual, 2)
			So(len(state.book.Asks), ShouldEqual, 2)
			So(state.book.Asks[0].Price, ShouldAlmostEqual, 100.01, 1e-12)
			So(state.book.Asks[0].Qty, ShouldAlmostEqual, 5, 1e-12)
		})
	})
}
