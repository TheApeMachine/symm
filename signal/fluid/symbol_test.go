package fluid

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

/*
TestFluidSymbolBookDerivedGrid proves the first complete book determines the
symbol lattice when no static fallback width is configured.
*/
func TestFluidSymbolBookDerivedGrid(t *testing.T) {
	Convey("Given no configured fluid grid width", t, withFluidGrid(map[string]any{
		"signals.fluid.tick_size":           0,
		"signals.fluid.grid_half_width":     0,
		"signals.volume_clock_bars_per_day": 288,
	}, func() {
		state, err := NewFluidSymbol("BTC/USD")

		Convey("When the first book snapshot arrives", func() {
			feedErr := state.FeedBook(kraken.BookData{
				Symbol: "BTC/USD",
				Type:   "snapshot",
				Bids: []kraken.BookLevel{
					testBookLevel("99.99", 5),
					testBookLevel("99.97", 3),
				},
				Asks: []kraken.BookLevel{
					testBookLevel("100.01", 5),
					testBookLevel("100.03", 3),
				},
			}, time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC))

			Convey("Then tick size and lattice width should come from the book", func() {
				So(err, ShouldBeNil)
				So(feedErr, ShouldBeNil)
				So(state.grid, ShouldNotBeNil)
				So(state.grid.tickSize, ShouldAlmostEqual, 0.02, 1e-12)
				So(state.grid.halfWidth, ShouldEqual, 2)
			})
		})
	}))
}

/*
TestPriceMemoryFromSamples proves price memory uses the complete normalized
path rather than reducing it to the latest step ratio.
*/
func TestPriceMemoryFromSamples(t *testing.T) {
	Convey("Given a normalized long-memory price path", t, func() {
		memory, err := priceMemoryFromSamples([]float64{100, 100.5, 101, 102})
		stepRatio := math.Abs(102-101) / (102 - 100)

		Convey("It should use fractional differencing instead of the one-step ratio", func() {
			So(err, ShouldBeNil)
			So(memory, ShouldBeGreaterThan, 0)
			So(math.Abs(memory-stepRatio), ShouldBeGreaterThan, 1e-6)
		})
	})
}

/*
TestFluidSymbolPartialBookUpdatePreservesRestingSide proves a sparse Kraken
delta changes only the supplied side of the retained symbol book.
*/
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

/*
TestFluidSymbolUpdateBeforeSnapshotWaits proves a sparse update cannot invent
the missing resting side before the authoritative snapshot arrives.
*/
func TestFluidSymbolUpdateBeforeSnapshotWaits(t *testing.T) {
	Convey("Given a fluid symbol without a book snapshot", t, withFluidGrid(nil, func() {
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
	}))
}
