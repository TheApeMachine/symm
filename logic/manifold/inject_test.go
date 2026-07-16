package manifold

import (
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
			MuX:            1,
			MuY:            1,
			AlphaXX:        0.4,
			AlphaYY:        0.2,
			AlphaXY:        0.04,
			AlphaYX:        0.06,
			Beta:           2,
			IntensityX:     4,
			IntensityY:     2,
			SpectralRadius: 0.35,
		},
	}
}

func TestStressAnisotropy(t *testing.T) {
	Convey("Given self-excitation asymmetry", t, func() {
		outcome := testOutcome()

		Convey("It should derive a dimensionless anisotropy ratio", func() {
			So(stressAnisotropy(outcome), ShouldAlmostEqual, 1.0/3.0)
		})
	})
}

func TestArrivalForcing(t *testing.T) {
	Convey("Given absolute conditional intensities and a fitted branching matrix", t, func() {
		outcome := testOutcome()
		buyPressure, sellPressure, ready := arrivalForcing(outcome, 0.01)

		Convey("It should preserve activity scale and directional cross-excitation", func() {
			So(ready, ShouldBeTrue)
			So(buyPressure, ShouldAlmostEqual, 0.0484)
			So(sellPressure, ShouldAlmostEqual, 0.0232)

			outcome.Fit.IntensityX *= 2
			outcome.Fit.IntensityY *= 2
			doubleBuy, doubleSell, doubleReady := arrivalForcing(outcome, 0.01)

			So(doubleReady, ShouldBeTrue)
			So(doubleBuy, ShouldAlmostEqual, 2*buyPressure)
			So(doubleSell, ShouldAlmostEqual, 2*sellPressure)
		})
	})
}

func TestIntegrationDeltaT(t *testing.T) {
	Convey("Given a chronological market interval", t, func() {
		config := testPhysicsConfig()

		Convey("It should use event time when it is tighter than the configured step", func() {
			deltaT := integrationDeltaT(config, time.Millisecond)

			So(deltaT, ShouldAlmostEqual, 0.001)
		})
	})
}

func TestEventInterval(t *testing.T) {
	Convey("Given successive market observations", t, func() {
		outcome := testOutcome()
		outcome.At = time.Unix(3, 0)

		Convey("It should derive solver time from event chronology", func() {
			So(
				eventInterval(testPhysicsConfig(), time.Unix(1, 0), outcome),
				ShouldEqual,
				2*time.Second,
			)
		})
	})
}
