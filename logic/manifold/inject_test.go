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
	Convey("Given empirical arrival intensity before Hawkes fit", t, func() {
		outcome := excitation.Outcome{
			BuyArrivalRate:  4,
			SellArrivalRate: 2,
			Readiness: excitation.Readiness{
				Observation: true,
				Intensity:   true,
			},
		}

		Convey("It should apply the observed side rates without invented coupling", func() {
			buyPressure, sellPressure, ready := arrivalForcing(outcome, 0.01)

			So(ready, ShouldBeTrue)
			So(buyPressure, ShouldAlmostEqual, 0.04)
			So(sellPressure, ShouldAlmostEqual, 0.02)
		})
	})

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

func TestApplyForcing(t *testing.T) {
	Convey("Given L3 carrier mass on both sides of the book", t, func() {
		config := testPhysicsConfig()
		outcome := testOutcome()
		oscillators := []pmanifold.Oscillator{
			{
				Amplitude: 0.2,
				PosX:      0.25,
				VelX:      -0.1,
			},
			{
				Amplitude: 0.3,
				PosX:      0.75,
				VelX:      0.1,
			},
			{
				Amplitude: 0.5,
				PosX:      0.80,
				VelX:      0.2,
			},
		}
		beforeSell := oscillators[0].Amplitude * oscillators[0].VelX
		beforeBuy := oscillators[1].Amplitude*oscillators[1].VelX +
			oscillators[2].Amplitude*oscillators[2].VelX

		characteristicSpeed, err := applyForcing(
			config, outcome, 10*time.Millisecond, oscillators,
		)

		Convey("It should conserve the fitted side impulse with carrier mass", func() {
			So(err, ShouldBeNil)
			So(characteristicSpeed, ShouldBeGreaterThan, 0)

			afterSell := oscillators[0].Amplitude * oscillators[0].VelX
			afterBuy := oscillators[1].Amplitude*oscillators[1].VelX +
				oscillators[2].Amplitude*oscillators[2].VelX

			So(afterSell-beforeSell, ShouldAlmostEqual, -0.0232)
			So(afterBuy-beforeBuy, ShouldAlmostEqual, 0.0484)
		})
	})
}

func BenchmarkApplyForcing(b *testing.B) {
	config := testPhysicsConfig()
	outcome := testOutcome()
	population := []pmanifold.Oscillator{
		{
			Amplitude: 0.2,
			PosX:      0.25,
			VelX:      -0.1,
		},
		{
			Amplitude: 0.3,
			PosX:      0.75,
			VelX:      0.1,
		},
		{
			Amplitude: 0.5,
			PosX:      0.80,
			VelX:      0.2,
		},
	}
	oscillators := make([]pmanifold.Oscillator, len(population))

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		copy(oscillators, population)

		if _, err := applyForcing(
			config, outcome, 10*time.Millisecond, oscillators,
		); err != nil {
			b.Fatal(err)
		}
	}
}

func TestIntegrationDeltaT(t *testing.T) {
	Convey("Given a chronological market interval", t, func() {
		config := testPhysicsConfig()

		Convey("It should use event time when it is tighter than the configured step", func() {
			deltaT := integrationDeltaT(config, time.Millisecond)

			So(deltaT, ShouldAlmostEqual, 0.001)
		})

		Convey("It should not exceed the configured stable integration step", func() {
			deltaT := integrationDeltaT(config, time.Second)

			So(deltaT, ShouldAlmostEqual, config.DeltaT)
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
