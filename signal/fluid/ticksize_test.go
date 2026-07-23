package fluid

import (
	"os"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestResolveBookTickSizePerSide(t *testing.T) {
	Convey("Given bid and ask ladders with intra-side steps", t, func() {
		tickSize, err := resolveBookTickSize(
			[]float64{100, 99.9, 99.8},
			[]float64{100.1, 100.2},
			0,
			0,
		)

		Convey("It should use the minimum intra-side increment", func() {
			So(err, ShouldBeNil)
			So(tickSize, ShouldAlmostEqual, 0.1, 1e-9)
		})
	})

	Convey("Given only touch prices on each side", t, func() {
		Convey("When an instrument increment fallback is available", func() {
			tickSize, err := resolveBookTickSize(
				[]float64{50000},
				[]float64{50001},
				0,
				0.1,
			)

			So(err, ShouldBeNil)
			So(tickSize, ShouldAlmostEqual, 0.1, 1e-9)
		})
	})
}

func TestSetInstrumentTickSize(t *testing.T) {
	Convey("Given a symbol waiting for tick resolution", t, withFluidGrid(nil, func() {
		registry := NewSyncRegistry()
		So(registry.SetInstrumentTickSize("BTC/EUR", 0.1), ShouldBeNil)

		state, err := registry.loadSymbol("BTC/EUR")
		So(err, ShouldBeNil)

		Convey("It should store the exchange increment for later book configuration", func() {
			So(state.instrumentTickSize, ShouldAlmostEqual, 0.1, 1e-12)
		})
	}))
}

func BenchmarkResolveBookTickSize(b *testing.B) {
	bids := []float64{100, 99.9, 99.8, 99.7, 99.6}
	asks := []float64{100.1, 100.2, 100.3, 100.4, 100.5}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = resolveBookTickSize(bids, asks, 0, 0)
	}
}

func TestResolveBookTickSizePrefersInstrument(t *testing.T) {
	Convey("Given noisy adjacent book prices", t, func() {
		tickSize, err := resolveBookTickSize(
			[]float64{100, 99.999999999},
			[]float64{100.1, 200},
			0.01,
			0,
		)

		Convey("Then it should use the exchange increment", func() {
			So(err, ShouldBeNil)
			So(tickSize, ShouldAlmostEqual, 0.01, 1e-12)
		})
	})
}

func TestGridHalfWidthFromBookPreservesObservedSpan(t *testing.T) {
	Convey("Given sparse levels spanning more ticks than resting rows", t, func() {
		bids := []kraken.BookLevel{
			{Price: *decimal.NewFromFloat64(99.98)},
			{Price: *decimal.NewFromFloat64(99.97)},
		}
		asks := []kraken.BookLevel{
			{Price: *decimal.NewFromFloat64(100.02)},
			{Price: *decimal.NewFromFloat64(100.03)},
		}

		Convey("Then it should preserve the complete observed tick span", func() {
			So(gridHalfWidthFromBook(bids, asks, 0.01), ShouldBeGreaterThanOrEqualTo, 3)
		})
	})
}

func TestGridHalfWidthSafeConversion(t *testing.T) {
	Convey("Given a span that would overflow int conversion", t, func() {
		bids := []kraken.BookLevel{
			{Price: *decimal.NewFromFloat64(1)},
			{Price: *decimal.NewFromFloat64(0.5)},
		}
		asks := []kraken.BookLevel{
			{Price: *decimal.NewFromFloat64(1.1)},
			{Price: *decimal.NewFromFloat64(1e12)},
		}

		derived := gridHalfWidthFromBook(bids, asks, 1e-18)

		Convey("Then it should refuse an unsafe width instead of wrapping negative", func() {
			So(derived, ShouldEqual, 0)
		})
	})
}

func TestNewFluidGridAcceptsConfiguredSpatialWidth(t *testing.T) {
	Convey("Given subscribed book depth", t, withFluidGrid(map[string]any{
		"signals.fluid.grid_half_width": 26,
	}, func() {
		Convey("When spatial width exceeds the number of subscribed rows", func() {
			grid, err := NewGrid()

			Convey("Then row count should not truncate the tick lattice", func() {
				So(err, ShouldBeNil)
				So(grid.halfWidth, ShouldEqual, 26)
			})
		})
	}))
}

func TestFluidBookSnapshotWithExchangeIncrement(t *testing.T) {
	Convey("Given a live MATIC book snapshot and exchange increment", t, withFluidGrid(map[string]any{
		"signals.fluid.tick_size":       0,
		"signals.fluid.grid_half_width": 0,
	}, func() {
		raw, readErr := os.ReadFile("../../tests/fixtures/book/fixtures/snapshot.json")
		So(readErr, ShouldBeNil)

		rows := kraken.NewBookDataSlice(raw)
		So(rows, ShouldNotBeEmpty)

		row := rows[0]
		row.Type = "snapshot"
		row.PriceIncrement = decimal.NewFromFloat64(0.0001)

		registry := NewSyncRegistry()
		book := NewBook(registry)

		Convey("When the fluid book signal ingests the snapshot", func() {
			measurements, err := book.Measure(row)

			Convey("Then it should configure the grid without panicking", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeNil)

				state, stateErr := registry.loadSymbol(row.Symbol)
				So(stateErr, ShouldBeNil)
				So(state.grid, ShouldNotBeNil)
				So(state.grid.halfWidth, ShouldBeLessThanOrEqualTo, 25)
			})
		})
	}))
}

func TestFluidSymbolConfigureTickFromBookPreservesWidth(t *testing.T) {
	Convey("Given a wide book and exchange increment", t, withFluidGrid(map[string]any{
		"signals.fluid.tick_size":       0,
		"signals.fluid.grid_half_width": 0,
	}, func() {
		state, err := NewFluidSymbol("ADV/USD")
		So(err, ShouldBeNil)

		state.instrumentTickSize = 0.01
		bids := []kraken.BookLevel{
			testBookLevel("100", 1),
			testBookLevel("50", 1),
		}
		asks := []kraken.BookLevel{
			testBookLevel("100.1", 1),
			testBookLevel("150", 1),
		}

		Convey("When the first snapshot configures the lattice", func() {
			configErr := state.configureTickFromBook(bids, asks)

			Convey("Then the half width should preserve the observed price span", func() {
				So(configErr, ShouldBeNil)
				So(state.grid, ShouldNotBeNil)
				So(state.grid.halfWidth, ShouldEqual, 5005)
			})
		})
	}))
}
