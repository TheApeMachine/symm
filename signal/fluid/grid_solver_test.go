package fluid

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluidGridIntegrateInterval(t *testing.T) {
	Convey("Given advection and diffusion at the grid's resolved scales", t, func() {
		grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, time.Second, 10)
		So(err, ShouldBeNil)

		dt := grid.integrationInterval.Seconds()
		resolvedVelocity := grid.spatialSpan() / dt

		for index := range grid.rho {
			grid.rho[index] = 1
			grid.velocity[index] = resolvedVelocity
		}

		grid.diffusionCoeff = grid.tickSize * grid.tickSize / dt
		stabilityRate := resolvedVelocity/grid.tickSize +
			2*grid.diffusionCoeff/(grid.tickSize*grid.tickSize)
		expected := int(math.Ceil(dt * stabilityRate))

		err = grid.integrateInterval(dt)

		Convey("It should derive a finite combined-stability substep count", func() {
			So(err, ShouldBeNil)
			So(grid.lastSubsteps, ShouldEqual, expected)
		})
	})
}

func BenchmarkFluidGridIntegrateInterval(b *testing.B) {
	grid, err := newFluidGrid(0.01, 10, 100*time.Millisecond, time.Second, 10)

	if err != nil {
		b.Fatal(err)
	}

	dt := grid.integrationInterval.Seconds()

	for index := range grid.rho {
		grid.rho[index] = 1
		grid.velocity[index] = grid.spatialSpan() / dt
	}

	grid.diffusionCoeff = grid.tickSize * grid.tickSize / dt
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := grid.integrateInterval(dt); err != nil {
			b.Fatal(err)
		}
	}
}
