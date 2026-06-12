package physics

import (
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestApplyDerivedGasParams(t *testing.T) {
	convey.Convey("Given production grid dimensions", t, func() {
		config := productionTestConfig()

		convey.Convey("It should use per-carrier envelope density for gas primitives", func() {
			convey.So(config.GasEnvelopeRhoMin, convey.ShouldAlmostEqual, config.RhoMin/float64(config.MaxModes), 1e-9)
			convey.So(config.GasPMin, convey.ShouldAlmostEqual, (config.Gamma-1.0)*config.GasEnvelopeRhoMin*config.CellVolume(), 1e-9)
		})

		convey.Convey("It should satisfy the von Neumann diffusion CFL bound", func() {
			convey.So(config.DiffusionCFL(), convey.ShouldBeLessThan, 1.0/6.0)
			convey.So(config.Validate(), convey.ShouldBeNil)
		})

		convey.Convey("It should place typical deposits inside the envelope scale", func() {
			typicalDeposit := config.RhoMin / float64(config.MaxModes)
			convey.So(typicalDeposit, convey.ShouldBeGreaterThan, config.GasEnvelopeRhoMin*0.5)
		})
	})
}

func TestValidateRejectsUnstableDiffusion(t *testing.T) {
	convey.Convey("Given an explicit diffusion CFL violation", t, func() {
		config := productionTestConfig()
		config.KThermal = config.RhoMin / config.DeltaT

		convey.Convey("It should fail validation", func() {
			convey.So(config.DiffusionCFL(), convey.ShouldBeGreaterThan, 1.0/6.0)
			convey.So(config.Validate(), convey.ShouldNotBeNil)
		})
	})
}

func BenchmarkDiffusionCFL(b *testing.B) {
	config := productionTestConfig()

	for b.Loop() {
		if math.IsNaN(config.DiffusionCFL()) {
			b.Fatal("diffusion CFL is NaN")
		}
	}
}
