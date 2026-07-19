package fluid

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestFluidSymbolBookDerivedGrid(t *testing.T) {
	Convey("Given no configured fluid grid width", t, func() {
		pinFluidViper(t, map[string]any{
			"signals.fluid.tick_size":            0,
			"signals.fluid.grid_half_width":      0,
			"signals.fluid.integration_interval": 100 * time.Millisecond,
			"signals.volume_clock_bars_per_day":  288,
		})

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
	})
}

func TestPriceMemoryFromSamples(t *testing.T) {
	Convey("Given a normalized long-memory price path", t, func() {
		memory := priceMemoryFromSamples([]float64{100, 100.5, 101, 102})
		stepRatio := math.Abs(102-101) / (102 - 100)

		Convey("It should use fractional differencing instead of the one-step ratio", func() {
			So(memory, ShouldBeGreaterThan, 0)
			So(math.Abs(memory-stepRatio), ShouldBeGreaterThan, 1e-6)
		})
	})
}
