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
	Convey("Given a symbol waiting for tick resolution", t, func() {
		registry := NewSyncRegistry()
		registry.SetInstrumentTickSize("BTC/EUR", 0.1)

		state := registry.loadSymbol("BTC/EUR")

		Convey("It should store the exchange increment for later book configuration", func() {
			So(state.instrumentTickSize, ShouldAlmostEqual, 0.1, 1e-12)
		})
	})
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

func TestCapGridHalfWidth(t *testing.T) {
	Convey("Given a derived width larger than subscribed depth", t, func() {
		capped := capGridHalfWidth(5000, 25, 10, 10)

		Convey("Then it should cap to the book depth budget", func() {
			So(capped, ShouldEqual, 10)
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

func TestNewFluidGridRejectsOversizedLattice(t *testing.T) {
	Convey("Given subscribed book depth", t, withFluidGrid(map[string]any{
		"signals.fluid.grid_half_width": 26,
	}, func() {
		Convey("When half width exceeds the depth budget", func() {
			_, err := NewGrid()

			Convey("Then grid construction should fail cleanly", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "book depth")
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

				state := registry.loadSymbol(row.Symbol)
				So(state.grid, ShouldNotBeNil)
				So(state.grid.halfWidth, ShouldBeLessThanOrEqualTo, 25)
			})
		})
	}))
}

func TestFluidSymbolConfigureTickFromBookCapsWidth(t *testing.T) {
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

			Convey("Then the half width should stay within subscribed depth", func() {
				So(configErr, ShouldBeNil)
				So(state.grid, ShouldNotBeNil)
				So(state.grid.halfWidth, ShouldEqual, 2)
			})
		})
	}))
}
