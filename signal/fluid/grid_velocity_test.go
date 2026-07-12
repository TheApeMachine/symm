package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluidGridInferVelocityField(t *testing.T) {
	Convey("Given equivalent density changes on differently scaled price lattices", t, func() {
		coarse, err := newFluidGrid(0.01, 2, 100*time.Millisecond, time.Second, 10)
		So(err, ShouldBeNil)

		fine, err := newFluidGrid(0.001, 2, 100*time.Millisecond, time.Second, 10)
		So(err, ShouldBeNil)

		prepareVelocityFixture(coarse)
		prepareVelocityFixture(fine)

		coarse.inferVelocityField(100, 0.1)
		fine.inferVelocityField(100, 0.1)

		Convey("It should scale inferred price velocity with the cell width", func() {
			index := coarse.midIndex
			So(coarse.velocity[index], ShouldAlmostEqual, fine.velocity[index]*10, 1e-12)
		})
	})
}

func TestFluidGridEstimateDiffusionCoefficient(t *testing.T) {
	Convey("Given geometrically equivalent shear fields on two price scales", t, func() {
		coarse, err := newFluidGrid(0.01, 2, 100*time.Millisecond, time.Second, 10)
		So(err, ShouldBeNil)

		fine, err := newFluidGrid(0.001, 2, 100*time.Millisecond, time.Second, 10)
		So(err, ShouldBeNil)

		for index := range coarse.velocity {
			fine.velocity[index] = float64(index) * fine.tickSize
			coarse.velocity[index] = float64(index) * coarse.tickSize
		}

		Convey("It should scale diffusivity with squared price distance", func() {
			coarseDiffusion := coarse.estimateDiffusionCoefficient()
			fineDiffusion := fine.estimateDiffusionCoefficient()
			So(coarseDiffusion, ShouldAlmostEqual, fineDiffusion*100, 1e-12)
		})
	})
}

func prepareVelocityFixture(grid *FluidGrid) {
	grid.prevMidPrice = 100

	for index := 1; index < len(grid.velocity)-1; index++ {
		grid.remappedRho[index] = 1
		grid.observedRho[index] = 2
		grid.sourceAccumulator[index] = 1
	}
}
