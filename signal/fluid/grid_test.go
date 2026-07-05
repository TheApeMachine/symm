package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
)

func setFluidGridConfig() {
	viper.Set("types.book_depth_levels", 10)
	viper.Set("signals.fluid.tick_size", 0.01)
	viper.Set("signals.fluid.grid_half_width", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	symbolConfigValue.Store(nil)
}

func TestFluidGridIngestBook(t *testing.T) {
	Convey("Given grid config and a book frame", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []kraken.BookLevel{{Price: 100.01, Qty: 4}}

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

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []kraken.BookLevel{{Price: 100.01, Qty: 4}}

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

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []kraken.BookLevel{{Price: 100.01, Qty: 4}}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestTrade(100.01, 2, at.Add(10*time.Millisecond)), ShouldBeNil)

		depletedAsks := []kraken.BookLevel{{Price: 100.01, Qty: 2}}
		So(grid.ingestBook(bids, depletedAsks, 100, at.Add(20*time.Millisecond)), ShouldBeNil)

		askIndex := grid.midIndex + 1

		Convey("It should attribute removal to execute rather than cancel", func() {
			So(grid.attributedExecuteAccumulator[askIndex], ShouldEqual, 2)
			So(grid.cancelAccumulator[askIndex], ShouldEqual, 0)
		})
	})
}

func TestFluidGridSparseDensityFilter(t *testing.T) {
	Convey("Given an isolated lattice density spike", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		index := grid.midIndex
		grid.observedRho[index] = 9
		before := densityMass(grid.observedRho)

		grid.filterSparseDensity(grid.observedRho)

		Convey("It should smooth local curvature without changing total density", func() {
			So(grid.observedRho[index], ShouldBeLessThan, 9)
			So(grid.observedRho[index-1], ShouldBeGreaterThan, 0)
			So(grid.observedRho[index+1], ShouldBeGreaterThan, 0)
			So(densityMass(grid.observedRho), ShouldAlmostEqual, before, 1e-9)
		})
	})
}

func TestFluidGridLagrangianRemapPreservesMassAtBoundary(t *testing.T) {
	Convey("Given prior density shifted outside the current lattice", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		prev := make([]float64, len(grid.rho))
		prev[grid.midIndex-2] = 3
		prev[grid.midIndex] = 5
		prev[grid.midIndex+2] = 7
		before := densityMass(prev)

		grid.lagrangianRemap(prev, 100, 101)

		Convey("It should accumulate right-shifted density at the upper boundary", func() {
			So(densityMass(grid.remappedRho), ShouldAlmostEqual, before, 1e-9)
			So(grid.remappedRho[len(grid.remappedRho)-1], ShouldBeGreaterThan, 0)
		})

		grid.lagrangianRemap(prev, 100, 99)

		Convey("It should accumulate left-shifted density at the lower boundary", func() {
			So(densityMass(grid.remappedRho), ShouldAlmostEqual, before, 1e-9)
			So(grid.remappedRho[0], ShouldBeGreaterThan, 0)
		})
	})
}

func TestFluidGridIngestBookResetsAfterIdleGap(t *testing.T) {
	Convey("Given a book gap longer than the idle threshold", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 500*time.Millisecond, 50)
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []kraken.BookLevel{{Price: 100.01, Qty: 4}}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestBook(bids, asks, 100, at.Add(time.Second)), ShouldBeNil)

		Convey("It should reset to the current book without replaying stale steps", func() {
			So(grid.stepCount, ShouldEqual, 0)
			So(grid.lastIntegrateAt, ShouldResemble, at.Add(time.Second))
			So(densityMass(grid.rho), ShouldAlmostEqual, densityMass(grid.filteredObservedRho), 1e-9)
		})
	})
}

func TestFluidGridIngestBookCapsCatchUpSteps(t *testing.T) {
	Convey("Given a non-idle gap larger than the configured step budget", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 3)
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []kraken.BookLevel{{Price: 100.01, Qty: 4}}
		nextAt := at.Add(time.Second)

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestBook(bids, asks, 100, nextAt), ShouldBeNil)

		Convey("It should bound catch-up work and advance to the live timestamp", func() {
			So(grid.stepCount, ShouldEqual, 3)
			So(grid.lastIntegrateAt, ShouldResemble, nextAt)
		})
	})
}

func TestFluidGridSpatialVelocity(t *testing.T) {
	Convey("Given asymmetric depth migration across the touch", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []kraken.BookLevel{{Price: 100.01, Qty: 4}}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)

		skewedBids := []kraken.BookLevel{
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

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{{Price: 99.99, Qty: 5}}
		asks := []kraken.BookLevel{{Price: 100.01, Qty: 4}}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
	})
}

func TestFluidGridMomentumDivergence(t *testing.T) {
	Convey("Given a touch density change after remap", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		index := grid.midIndex
		grid.observedRho[index] = 10
		grid.remappedRho[index] = 8

		grid.measureMidDivergence()

		Convey("It should report normalized touch divergence", func() {
			So(grid.midVelocityDivergence(), ShouldAlmostEqual, 0.2, 1e-12)
		})
	})
}

func TestFluidGridRK2NeumannBoundary(t *testing.T) {
	Convey("Given non-uniform boundary density with advection", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		for index := range grid.rho {
			grid.rho[index] = 1
			grid.velocity[index] = 0.05
		}

		grid.rho[0] = 5
		grid.rho[len(grid.rho)-1] = 5

		grid.integrateRK2(0.05)

		Convey("It should enforce zero-gradient boundaries after integration", func() {
			lastIndex := len(grid.rho) - 1
			So(grid.rho[0], ShouldEqual, grid.rho[1])
			So(grid.rho[lastIndex], ShouldEqual, grid.rho[lastIndex-1])
		})
	})
}

func TestFluidGridReplenishmentRatio(t *testing.T) {
	Convey("Given matched add and execute rates at the touch", t, func() {
		setFluidGridConfig()

		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)
		So(err, ShouldBeNil)

		grid.sources = make([]float64, len(grid.rho))
		grid.addAccumulator[grid.midIndex] = 8
		grid.attributedExecuteAccumulator[grid.midIndex] = 8
		grid.sourceAccumulator[grid.midIndex] = 0
		grid.sources[grid.midIndex-1] = 4
		grid.sources[grid.midIndex+1] = -4

		grid.measureReplenishment(0.1, 0.02)

		Convey("It should use replenished over consumed as viscosity", func() {
			So(grid.viscosity(), ShouldEqual, 1)
			So(grid.midAddRateAtTouch(), ShouldBeGreaterThan, 0)
			So(grid.midExecuteRateAtTouch(), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkFluidGridIntegrateRK2(b *testing.B) {
	setFluidGridConfig()

	grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, 5*time.Second, 50)

	if err != nil {
		b.Fatal(err)
	}

	for index := range grid.rho {
		grid.rho[index] = 1
		grid.velocity[index] = 0.05
	}

	b.ResetTimer()

	for b.Loop() {
		grid.integrateRK2(0.05)
	}
}

func TestRusanovFlux1D(t *testing.T) {
	Convey("Given equal states across a face", t, func() {
		flux := rusanovFlux1D(10, 10, 5, 5, 2)

		Convey("It should return the common flux", func() {
			So(flux, ShouldEqual, 10)
		})
	})
}
