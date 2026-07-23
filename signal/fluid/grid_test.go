package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestFluidGridIngestBook(t *testing.T) {
	Convey("Given grid config and a book frame", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}

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
			So(grid.midVelocityCurvature(), ShouldBeGreaterThanOrEqualTo, 0)
			So(grid.turbulenceIntensity(), ShouldBeGreaterThanOrEqualTo, 0)
		})
	}))
}

func TestFluidGridRK2ZeroSource(t *testing.T) {
	Convey("Given a stationary book across one integration interval", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)

		before := append([]float64(nil), grid.rho...)

		nextAt := at.Add(100 * time.Millisecond)
		So(grid.ingestBook(bids, asks, 100, nextAt), ShouldBeNil)

		Convey("It should leave density unchanged when sources and velocity are zero", func() {
			for index := range before {
				So(grid.rho[index], ShouldAlmostEqual, before[index], 1e-9)
			}
		})
	}))
}

func TestFluidGridSourceDecomposition(t *testing.T) {
	Convey("Given a trade followed by book depletion at the touch", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestTrade(100.01, 2, at.Add(10*time.Millisecond)), ShouldBeNil)

		depletedAsks := []kraken.BookLevel{testBookLevel("100.01", 2)}
		So(grid.ingestBook(bids, depletedAsks, 100, at.Add(20*time.Millisecond)), ShouldBeNil)

		askIndex := grid.midIndex + 1

		Convey("It should attribute removal to execute rather than cancel", func() {
			So(grid.attributedExecuteAccumulator[askIndex], ShouldEqual, 2)
			So(grid.cancelAccumulator[askIndex], ShouldEqual, 0)
		})
	}))

	Convey("Given execution depletion spanning multiple integration steps", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}
		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestTrade(100.01, 2, at.Add(10*time.Millisecond)), ShouldBeNil)
		depletedAsks := []kraken.BookLevel{testBookLevel("100.01", 2)}
		So(grid.ingestBook(bids, depletedAsks, 100, at.Add(time.Second)), ShouldBeNil)

		Convey("It should retain the observed execution through catch-up integration", func() {
			So(grid.midExecuteRateAtTouch(), ShouldEqual, 2)
			So(grid.midSourceBalance(), ShouldEqual, -2)
		})
	}))

	Convey("Given an execution immediately replenished at the same price", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}
		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestTrade(100.01, 2, at.Add(10*time.Millisecond)), ShouldBeNil)
		So(grid.ingestBook(bids, asks, 100, at.Add(time.Second)), ShouldBeNil)

		Convey("It should reconcile unchanged depth as matched execution and addition", func() {
			So(grid.midExecuteRateAtTouch(), ShouldEqual, 2)
			So(grid.midAddRateAtTouch(), ShouldEqual, 2)
			So(grid.midSourceBalance(), ShouldEqual, 0)
			So(grid.viscosity(), ShouldEqual, 1)
		})
	}))
}

func TestFluidGridSparseDensityFilter(t *testing.T) {
	Convey("Given an isolated lattice density spike", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
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
	}))
}

func TestFluidGridLagrangianRemapPreservesMassAtBoundary(t *testing.T) {
	Convey("Given prior density shifted outside the current lattice", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
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
	}))
}

func TestFluidGridIngestBookResetsAfterIdleGap(t *testing.T) {
	Convey("Given a book gap longer than the idle threshold", t, withFluidGrid(map[string]any{
		"signals.fluid.idle_threshold": 500 * time.Millisecond,
	}, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestBook(bids, asks, 100, at.Add(time.Second)), ShouldBeNil)

		Convey("It should reset to the current book without replaying stale steps", func() {
			So(grid.stepCount, ShouldEqual, 0)
			So(grid.lastIntegrateAt, ShouldResemble, at.Add(time.Second))
			So(densityMass(grid.rho), ShouldAlmostEqual, densityMass(grid.filteredObservedRho), 1e-9)
		})
	}))
}

func TestFluidGridIngestBookCapsCatchUpSteps(t *testing.T) {
	Convey("Given a non-idle gap larger than the configured step budget", t, withFluidGrid(map[string]any{
		"signals.fluid.max_integration_steps": 3,
	}, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}
		nextAt := at.Add(time.Second)

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestBook(bids, asks, 100, nextAt), ShouldBeNil)

		Convey("It should bound catch-up work and advance to the live timestamp", func() {
			So(grid.stepCount, ShouldEqual, 3)
			So(grid.lastIntegrateAt, ShouldResemble, nextAt)
		})
	}))
}

func TestFluidGridSpatialVelocity(t *testing.T) {
	Convey("Given asymmetric depth migration across the touch", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)

		skewedBids := []kraken.BookLevel{
			testBookLevel("99.99", 8),
			testBookLevel("99.98", 1),
		}
		So(grid.ingestBook(skewedBids, asks, 100, at.Add(50*time.Millisecond)), ShouldBeNil)

		grid.inferVelocityField(100, 0.05)

		Convey("It should infer distinct velocities across cells", func() {
			So(grid.velocity[grid.midIndex-1], ShouldNotEqual, grid.velocity[grid.midIndex+1])
		})
	}))
}

func TestFluidGridIngestBookSkipsDuplicateTimestamp(t *testing.T) {
	Convey("Given a book frame already ingested at the same timestamp", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		at := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		bids := []kraken.BookLevel{testBookLevel("99.99", 5)}
		asks := []kraken.BookLevel{testBookLevel("100.01", 4)}

		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
		So(grid.ingestBook(bids, asks, 100, at), ShouldBeNil)
	}))
}

func TestFluidGridMomentumDivergence(t *testing.T) {
	Convey("Given a density gradient advected by a uniform touch-region velocity field", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		index := grid.midIndex
		grid.rho[index-1] = 8
		grid.rho[index] = 10
		grid.rho[index+1] = 12
		grid.velocity[index-1] = 1
		grid.velocity[index] = 1
		grid.velocity[index+1] = 1

		grid.measureMidDivergence()

		Convey("It should report a positive divergence: rightward flow exports mass out of a cell sitting in rising density", func() {
			So(grid.midVelocityDivergence(), ShouldAlmostEqual, 20, 1e-9)
		})
	}))

	Convey("Given the same density gradient with the flow direction reversed", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		index := grid.midIndex
		grid.rho[index-1] = 8
		grid.rho[index] = 10
		grid.rho[index+1] = 12
		grid.velocity[index-1] = -1
		grid.velocity[index] = -1
		grid.velocity[index+1] = -1

		grid.measureMidDivergence()

		Convey("It should flip sign: leftward flow now imports mass into the touch cell", func() {
			So(grid.midVelocityDivergence(), ShouldAlmostEqual, -20, 1e-9)
		})
	}))

	Convey("Given an empty midpoint between resting touch density", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		index := grid.midIndex
		grid.rho[index-1] = 8
		grid.rho[index+1] = 12
		grid.observedRho[index-1] = 8
		grid.observedRho[index+1] = 12
		grid.velocity[index-1] = 1
		grid.velocity[index] = 1
		grid.velocity[index+1] = 1

		grid.measureMidDivergence()

		Convey("It should normalize against observed depth instead of an empty-cell floor", func() {
			So(grid.midVelocityDivergence(), ShouldAlmostEqual, -80, 1e-9)
		})
	}))
}

func TestFluidGridRK2NeumannBoundary(t *testing.T) {
	Convey("Given non-uniform boundary density with advection", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
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
	}))
}

func TestFluidGridReplenishmentRatio(t *testing.T) {
	Convey("Given matched add and execute rates at the touch", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)

		grid.sources = make([]float64, len(grid.rho))
		grid.addAccumulator[grid.midIndex] = 8
		grid.attributedExecuteAccumulator[grid.midIndex] = 8
		grid.sourceAccumulator[grid.midIndex] = 0
		grid.sources[grid.midIndex-1] = 4
		grid.sources[grid.midIndex+1] = -4

		grid.measureReplenishment(0.02, grid.integrationInterval.Seconds())

		Convey("It should use replenished over consumed as viscosity", func() {
			So(grid.viscosity(), ShouldEqual, 1)
			So(grid.midAddRateAtTouch(), ShouldBeGreaterThan, 0)
			So(grid.midExecuteRateAtTouch(), ShouldBeGreaterThan, 0)
		})
	}))
}

func TestFluidGridReplenishmentRequiresConsumption(t *testing.T) {
	Convey("Given touch replenishment without observed consumption", t, withFluidGrid(nil, func() {
		grid, err := NewGrid()
		So(err, ShouldBeNil)
		grid.addAccumulator[grid.midIndex] = 4
		grid.sourceAccumulator[grid.midIndex] = 4

		grid.measureReplenishment(0.02, grid.integrationInterval.Seconds())

		Convey("It should not mix replenished quantity into the viscosity ratio", func() {
			So(grid.viscosity(), ShouldEqual, 0)
		})
	}))
}

func BenchmarkFluidGridIntegrateRK2(b *testing.B) {
	configureFluidBenchmark(fluidGridSettings(nil))

	grid, err := NewGrid()

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

func BenchmarkFluidGridIngestBook(b *testing.B) {
	configureFluidBenchmark(fluidGridSettings(nil))

	grid, err := NewGrid()

	if err != nil {
		b.Fatal(err)
	}

	at := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)
	bids := []kraken.BookLevel{testBookLevel("99.99", 4)}
	asks := []kraken.BookLevel{testBookLevel("100.01", 6)}

	if err := grid.ingestBook(bids, asks, 100, at); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		at = at.Add(grid.integrationInterval)
		bids[0].Qty, asks[0].Qty = asks[0].Qty, bids[0].Qty

		if err := grid.ingestBook(bids, asks, 100, at); err != nil {
			b.Fatal(err)
		}
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
