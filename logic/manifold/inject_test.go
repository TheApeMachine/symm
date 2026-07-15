package manifold

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

func testPhysicsConfig() pmanifold.Config {
	config := pmanifold.Config{
		GridX: 8, GridY: 8, GridZ: 8,
		DomainX: 1, DomainY: 1, DomainZ: 1,
		DeltaT: 0.01, Gamma: 5.0 / 3.0, MaxModes: 4,
	}
	pmanifold.ApplyDerivedGasParams(&config)
	pmanifold.DefaultMarketGasBoundaries().Apply(&config)

	return config
}

func testOutcome() excitation.Outcome {
	return excitation.Outcome{
		At:       time.Unix(1, 0),
		Horizon:  time.Second,
		Maturity: 0.5,
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
		},
		Fit: hawkes.BivariateFit{
			AlphaXX:        4,
			AlphaYY:        2,
			AlphaXY:        0.4,
			AlphaYX:        0.6,
			Beta:           2,
			IntensityX:     4,
			IntensityY:     2,
			SpectralRadius: 0.35,
		},
	}
}

func TestHawkesCells(t *testing.T) {
	Convey("Given a bivariate Hawkes outcome and grid config", t, func() {
		config := testPhysicsConfig()
		outcome := testOutcome()

		Convey("It should map intensities and branching into bounded cell indices", func() {
			buyCellX, sellCellX, cellY, cellZ := hawkesCells(
				config, outcome, 4, 2, 6,
			)

			So(buyCellX, ShouldEqual, 5)
			So(sellCellX, ShouldEqual, 2)
			So(cellY, ShouldBeLessThan, config.GridY)
			So(cellZ, ShouldEqual, 2)
		})
	})
}

func TestStressAnisotropy(t *testing.T) {
	Convey("Given self-excitation asymmetry", t, func() {
		outcome := testOutcome()

		Convey("It should derive a dimensionless anisotropy ratio", func() {
			So(stressAnisotropy(outcome), ShouldAlmostEqual, 1.0/3.0)
		})
	})
}

func TestBuildOscillators(t *testing.T) {
	Convey("Given Hawkes intensities", t, func() {
		config := testPhysicsConfig()
		outcome := testOutcome()
		oscillators := buildOscillators(config, outcome, 4, 2, 6)

		Convey("It should produce two finite buy and sell oscillator modes", func() {
			So(len(oscillators), ShouldEqual, 2)
			So(oscillators[0].Omega, ShouldEqual, outcome.Fit.Beta)
			So(oscillators[0].Amplitude, ShouldAlmostEqual, 4.0/6.0)
			So(oscillators[1].Amplitude, ShouldAlmostEqual, 2.0/6.0)
			So(math.IsNaN(oscillators[0].PosX), ShouldBeFalse)
			So(math.IsNaN(oscillators[1].VelX), ShouldBeFalse)
		})
	})
}

func TestIntegrationDeltaT(t *testing.T) {
	Convey("Given a Hawkes decay rate", t, func() {
		config := testPhysicsConfig()
		outcome := testOutcome()

		Convey("It should choose the advective limit when it is tighter", func() {
			deltaT := integrationDeltaT(config, outcome)

			So(deltaT, ShouldBeGreaterThan, 0)
			So(deltaT, ShouldBeLessThanOrEqualTo, config.DeltaT)
		})
	})
}
