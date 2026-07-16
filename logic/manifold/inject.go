package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
inject applies Hawkes forcing to the authoritative L3 carriers before installing
them. PIC then deposits each carrier's mass and forced momentum together.
*/
func inject(
	handle *pmanifold.Solver,
	config pmanifold.Config,
	outcome excitation.Outcome,
	interval time.Duration,
	oscillators []pmanifold.Oscillator,
) error {
	if handle == nil {
		return errnie.Err(
			errnie.Internal,
			"manifold: solver handle is not initialized",
			nil,
		)
	}

	if len(oscillators) == 0 {
		return errnie.Err(
			errnie.Internal,
			"manifold: oscillator population is empty",
			nil,
		)
	}

	if err := applyForcing(config, outcome, interval, oscillators); err != nil {
		return err
	}

	if err := handle.SetOscillators(oscillators); err != nil {
		return errnie.Err(
			errnie.Internal,
			"manifold: failed to set L3 carriers",
			err,
		)
	}

	return nil
}

/*
applyForcing distributes one event-time Hawkes impulse across the L3 carrier
mass. Applying it as velocity keeps the deposited gas state conservative.
*/
func applyForcing(
	config pmanifold.Config,
	outcome excitation.Outcome,
	interval time.Duration,
	oscillators []pmanifold.Oscillator,
) error {
	buyPressure, sellPressure, ready := arrivalForcing(
		outcome, integrationDeltaT(config, interval),
	)

	if !ready {
		return errnie.Err(
			errnie.Validation,
			"manifold: Hawkes forcing is not ready",
			nil,
		)
	}

	midpoint := config.DomainX / 2
	buyMass, sellMass := 0.0, 0.0

	for _, oscillator := range oscillators {
		if oscillator.PosX >= midpoint {
			buyMass += oscillator.Amplitude
			continue
		}

		sellMass += oscillator.Amplitude
	}

	if buyMass <= 0 || sellMass <= 0 {
		return errnie.Err(
			errnie.Validation,
			"manifold: L3 carriers require both book sides for forcing",
			nil,
		)
	}

	for index := range oscillators {
		oscillator := &oscillators[index]
		pressure, mass := buyPressure, buyMass

		if oscillator.PosX < midpoint {
			pressure, mass = -sellPressure, sellMass
		}

		oscillator.VelX += pressure / mass
	}

	return nil
}

/*
arrivalForcing advances the fitted branching matrix by one solver interval so
self- and cross-excitation contribute to the next absolute arrival pressure.
*/
func arrivalForcing(
	outcome excitation.Outcome,
	deltaT float64,
) (buyPressure float64, sellPressure float64, ready bool) {
	buyIntensity, sellIntensity := intensities(outcome)
	beta := outcome.Fit.Beta

	if !outcome.Readiness.HawkesFit || !outcome.Fit.Valid() || beta <= 0 || deltaT <= 0 ||
		buyIntensity < 0 || sellIntensity < 0 {
		return 0, 0, false
	}

	buyPressure = (buyIntensity +
		(outcome.Fit.AlphaXX*buyIntensity+outcome.Fit.AlphaXY*sellIntensity)/beta) *
		deltaT
	sellPressure = (sellIntensity +
		(outcome.Fit.AlphaYX*buyIntensity+outcome.Fit.AlphaYY*sellIntensity)/beta) *
		deltaT

	return buyPressure, sellPressure,
		finiteNonNegative(buyPressure) && finiteNonNegative(sellPressure)
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func intensities(outcome excitation.Outcome) (buyIntensity float64, sellIntensity float64) {
	if outcome.Readiness.HawkesFit {
		return outcome.Fit.IntensityX, outcome.Fit.IntensityY
	}

	return outcome.BuyArrivalRate, outcome.SellArrivalRate
}

func stressAnisotropy(outcome excitation.Outcome) float64 {
	selfSum := outcome.Fit.AlphaXX + outcome.Fit.AlphaYY

	if selfSum <= 0 {
		return 0
	}

	return math.Abs(outcome.Fit.AlphaXX-outcome.Fit.AlphaYY) / selfSum
}

func integrationDeltaT(config pmanifold.Config, interval time.Duration) float64 {
	if interval > 0 {
		return interval.Seconds()
	}

	return config.DeltaT
}

func eventInterval(
	config pmanifold.Config,
	previous time.Time,
	outcome excitation.Outcome,
) time.Duration {
	if !previous.IsZero() && outcome.At.After(previous) {
		return outcome.At.Sub(previous)
	}

	return time.Duration(config.DeltaT * float64(time.Second))
}
