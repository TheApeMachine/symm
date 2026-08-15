package manifold

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
			convey.So(config.DiffusionCFL(), convey.ShouldBeLessThan, 0.5)
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
			convey.So(config.DiffusionCFL(), convey.ShouldBeGreaterThan, 0.5)
			convey.So(config.Validate(), convey.ShouldNotBeNil)
		})
	})
}

func TestConfigRuntimeControls(t *testing.T) {
	convey.Convey("Given a validated manifold config", t, func() {
		config := productionTestConfig()

		convey.Convey("It should expose the integration controls used by the solver", func() {
			controls := config.RuntimeControls()

			convey.So(controls.DeltaT, convey.ShouldEqual, config.DeltaT)
			convey.So(controls.MetabolicRate, convey.ShouldEqual, config.MetabolicRate())
			convey.So(controls.GInteraction, convey.ShouldEqual, config.GInteraction())
			convey.So(controls.EnergyDecay, convey.ShouldEqual, config.EnergyDecay())
			convey.So(controls.Validate(), convey.ShouldBeNil)
		})
	})
}

func TestRuntimeControlsValidate(t *testing.T) {
	convey.Convey("Given invalid runtime controls", t, func() {
		controls := productionTestConfig().RuntimeControls()
		controls.TopdownPhaseScale = math.Inf(1)

		convey.Convey("It should reject non-finite prior values", func() {
			convey.So(controls.Validate(), convey.ShouldNotBeNil)
		})
	})
}

func productionTestConfig() Config {
	tickSize := 0.01
	halfWidth := 32
	gamma := 5.0 / 3.0
	deltaT := 0.1

	config := Config{
		GridX:    32,
		GridY:    3,
		GridZ:    16,
		DomainX:  float64(halfWidth*2+1) * tickSize,
		DomainY:  3,
		DomainZ:  16,
		DeltaT:   deltaT,
		Gamma:    gamma,
		MaxModes: 128,
	}

	config = config.stableGasTestConfig(0, 1)
	DefaultMarketGasBoundaries().Apply(&config)

	return config
}

func (config Config) stableGasTestConfig(velocity, specificInternalEnergy float64) Config {
	ApplyDerivedGasParams(&config)

	soundSpeed := math.Sqrt(config.Gamma * (config.Gamma - 1) * specificInternalEnergy)
	rarefactionSpeed := velocity + 2*soundSpeed/(config.Gamma-1)
	config.DeltaT = config.AdvectiveDeltaT(rarefactionSpeed)
	ApplyDerivedGasParams(&config)

	return config
}

func (config Config) testCellCenter(
	cellX, cellY, cellZ uint32,
) (float64, float64, float64) {
	return (float64(cellX) + 0.5) * config.DomainX / float64(config.GridX),
		(float64(cellY) + 0.5) * config.DomainY / float64(config.GridY),
		(float64(cellZ) + 0.5) * config.DomainZ / float64(config.GridZ)
}

func BenchmarkDiffusionCFL(b *testing.B) {
	config := productionTestConfig()

	for b.Loop() {
		if math.IsNaN(config.DiffusionCFL()) {
			b.Fatal("diffusion CFL is NaN")
		}
	}
}

func TestConfigAdvectiveDeltaT(t *testing.T) {
	convey.Convey("Given an anisotropic grid and a finite characteristic speed", t, func() {
		config := Config{
			GridX: 10, GridY: 2, GridZ: 5,
			DomainX: 1, DomainY: 2, DomainZ: 1,
		}

		deltaT := config.AdvectiveDeltaT(2)

		convey.Convey("It should invert the exact multidimensional Courant rate", func() {
			convey.So(deltaT, convey.ShouldAlmostEqual, 1.0/(2.0*(10.0+1.0+5.0)))
		})
	})
}
