package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadSymbolConfigWithoutTickOverride(t *testing.T) {
	Convey("Given no signals.fluid.tick_size override", t, withFluidConfig(map[string]any{
		"signals.fluid.tick_size":            0,
		"signals.fluid.grid_half_width":      10,
		"signals.fluid.integration_interval": 0,
		"signals.volume_clock_bars_per_day":  288,
	}, func() {
		Convey("When loadSymbolConfig is called", func() {
			config, err := loadSymbolConfig()

			Convey("It should defer tick resolution to book or instrument catalog", func() {
				So(err, ShouldBeNil)
				So(config.tickSizeFallback, ShouldEqual, 0)
				So(config.gridHalfWidth, ShouldEqual, 10)
			})
		})
	}))
}

func TestNewFluidSymbolDefersGridUntilBookTick(t *testing.T) {
	Convey("Given no signals.fluid.tick_size override", t, withFluidConfig(map[string]any{
		"signals.fluid.tick_size":            0,
		"signals.fluid.grid_half_width":      10,
		"signals.fluid.integration_interval": 100 * time.Millisecond,
		"signals.volume_clock_bars_per_day":  288,
	}, func() {
		symbol := "BTC/USD"
		fixture := symbolBookFixture{symbol: symbol}
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		Convey("When NewFluidSymbol is called before book data arrives", func() {
			state, err := NewFluidSymbol(symbol)

			Convey("It should construct without failing and defer grid creation", func() {
				So(err, ShouldBeNil)
				So(state, ShouldNotBeNil)
				So(state.grid, ShouldBeNil)
			})

			Convey("When the first book snapshot arrives", func() {
				So(state.FeedBook(fixture.snapshot(99.99, 5, 100.01, 5), feedAt), ShouldBeNil)

				Convey("It should derive tick size from book levels", func() {
					So(state.grid, ShouldNotBeNil)
					So(state.grid.tickSize, ShouldAlmostEqual, 0.01, 1e-9)
				})
			})
		})
	}))
}
