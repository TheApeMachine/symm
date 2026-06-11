package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func setFluidGridConfig() {
	viper.Set("signals.fluid.tick_size", 0.01)
	viper.Set("signals.fluid.grid_half_width", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
}

func TestFluidGridIngestBook(t *testing.T) {
	Convey("Given grid config and a book frame", t, func() {
		setFluidGridConfig()

		grid, err := NewFluidGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []krakenmarket.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []krakenmarket.BookLevel{{Price: 100.01, Qty: 4}}

		ingestErr := grid.ingestBook(bids, asks, 100, at)

		Convey("It should accept the first projection", func() {
			So(ingestErr, ShouldBeNil)
		})

		nextAt := at.Add(100 * time.Millisecond)
		ingestErr = grid.ingestBook(bids, asks, 100, nextAt)

		Convey("It should integrate on the fixed lattice interval", func() {
			So(ingestErr, ShouldBeNil)
			So(grid.ready(), ShouldBeTrue)
			So(grid.reynolds(0.02), ShouldBeGreaterThanOrEqualTo, 0)
			So(grid.midVorticity(), ShouldBeGreaterThanOrEqualTo, 0)
			So(grid.turbulenceIntensity(), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestFluidGridRK2ZeroSource(t *testing.T) {
	Convey("Given a stationary book across one integration interval", t, func() {
		setFluidGridConfig()

		grid, err := NewFluidGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []krakenmarket.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []krakenmarket.BookLevel{{Price: 100.01, Qty: 4}}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)

		before := append([]float64(nil), grid.rho...)

		nextAt := at.Add(100 * time.Millisecond)
		So(grid.ingestBook(bids, asks, 100, nextAt), ShouldBeNil)

		Convey("It should leave density unchanged when sources and velocity are zero", func() {
			for index := range before {
				So(grid.rho[index], ShouldAlmostEqual, before[index], 1e-9)
			}
		})
	})
}

func TestFluidGridSourceDecomposition(t *testing.T) {
	Convey("Given a trade followed by book depletion at the touch", t, func() {
		setFluidGridConfig()

		grid, err := NewFluidGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []krakenmarket.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []krakenmarket.BookLevel{{Price: 100.01, Qty: 4}}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestTrade(100.01, 2, at.Add(10*time.Millisecond)), ShouldBeNil)

		depletedAsks := []krakenmarket.BookLevel{{Price: 100.01, Qty: 2}}
		So(grid.ingestBook(bids, depletedAsks, 100, at.Add(20*time.Millisecond)), ShouldBeNil)

		askIndex := grid.midIndex + 1

		Convey("It should attribute removal to execute rather than cancel", func() {
			So(grid.attributedExecuteAccumulator[askIndex], ShouldEqual, 2)
			So(grid.cancelAccumulator[askIndex], ShouldEqual, 0)
		})
	})
}

func TestFluidGridSpatialVelocity(t *testing.T) {
	Convey("Given asymmetric depth migration across the touch", t, func() {
		setFluidGridConfig()

		grid, err := NewFluidGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []krakenmarket.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []krakenmarket.BookLevel{{Price: 100.01, Qty: 4}}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)

		skewedBids := []krakenmarket.BookLevel{
			{Price: 99.99, Qty: 8},
			{Price: 99.98, Qty: 1},
		}
		So(grid.ingestBook(skewedBids, asks, 100, at.Add(50*time.Millisecond)), ShouldBeNil)

		grid.inferVelocityField(100, 0.05)

		Convey("It should infer distinct velocities across cells", func() {
			So(grid.velocity[grid.midIndex-1], ShouldNotEqual, grid.velocity[grid.midIndex+1])
		})
	})
}

func TestFluidGridIngestBookSkipsDuplicateTimestamp(t *testing.T) {
	Convey("Given a book frame already ingested at the same timestamp", t, func() {
		setFluidGridConfig()

		grid, err := NewFluidGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []krakenmarket.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []krakenmarket.BookLevel{{Price: 100.01, Qty: 4}}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
	})
}

func TestRusanovFlux1D(t *testing.T) {
	Convey("Given equal states across a face", t, func() {
		flux := rusanovFlux1D(10, 10, 5, 5, 2)

		Convey("It should return the common flux", func() {
			So(flux, ShouldEqual, 10)
		})
	})
}
