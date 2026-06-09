package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestFluidGridStep(t *testing.T) {
	Convey("Given grid config and a book frame", t, func() {
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)

		grid, err := NewFluidGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []krakenmarket.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []krakenmarket.BookLevel{{Price: 100.01, Qty: 4}}

		stepErr := grid.step(bids, asks, 100, at)

		Convey("It should accept the first projection", func() {
			So(stepErr, ShouldBeNil)
		})

		nextAt := at.Add(time.Second)
		stepErr = grid.step(bids, asks, 100, nextAt)

		Convey("It should advance the field on the next frame", func() {
			So(stepErr, ShouldBeNil)
			So(grid.ready(), ShouldBeTrue)
			So(grid.reynolds(0.02), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}
