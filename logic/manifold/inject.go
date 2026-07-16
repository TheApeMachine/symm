package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
inject installs the forced authoritative L3 carriers so PIC deposits each
carrier's mass and momentum together on the next solver step.
*/
func inject(
	handle *pmanifold.Solver,
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
mass and returns the fastest resulting rarefaction. Applying pressure as
velocity keeps the deposited gas state conservative, while the rarefaction
sets the gas solver's advective stability limit.
*/
func applyForcing(
	config pmanifold.Config,
	outcome excitation.Outcome,
	interval time.Duration,
	oscillators []pmanifold.Oscillator,
) (float64, error) {
	buyPressure, sellPressure, ready := arrivalForcing(
		outcome, integrationDeltaT(config, interval),
	)

	if !ready {
		return 0, errnie.Err(
			errnie.Validation,
			"manifold: arrival forcing is not ready",
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
		return 0, errnie.Err(
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

	characteristicSpeed := 0.0

	for _, oscillator := range oscillators {
		velocity := math.Sqrt(
			oscillator.VelX*oscillator.VelX +
				oscillator.VelY*oscillator.VelY +
				oscillator.VelZ*oscillator.VelZ,
		)
		specificInternalEnergy := oscillator.Heat / oscillator.Amplitude
		soundSpeed := math.Sqrt(
			config.Gamma * (config.Gamma - 1) * specificInternalEnergy,
		)
		rarefactionSpeed := velocity + 2*soundSpeed/(config.Gamma-1)

		if !finiteNonNegative(rarefactionSpeed) {
			return 0, errnie.Err(
				errnie.Validation,
				"manifold: carrier characteristic speed is not finite",
				nil,
			)
		}

		if rarefactionSpeed > characteristicSpeed {
			characteristicSpeed = rarefactionSpeed
		}
	}

	if characteristicSpeed <= 0 {
		return 0, errnie.Err(
			errnie.Validation,
			"manifold: carrier characteristic speed must be positive",
			nil,
		)
	}

	return characteristicSpeed, nil
}

/*
arrivalForcing converts observed arrival intensity into the next absolute
pressure impulse. Once fitted, the Hawkes branching matrix adds self- and
cross-excitation; before fit, the empirical side rates remain valid forcing.
*/
func arrivalForcing(
	outcome excitation.Outcome,
	deltaT float64,
) (buyPressure float64, sellPressure float64, ready bool) {
	buyIntensity, sellIntensity := intensities(outcome)

	if !outcome.Readiness.Intensity || deltaT <= 0 ||
		buyIntensity < 0 || sellIntensity < 0 {
		return 0, 0, false
	}

	if !outcome.Readiness.HawkesFit {
		buyPressure = buyIntensity * deltaT
		sellPressure = sellIntensity * deltaT

		return buyPressure, sellPressure,
			finiteNonNegative(buyPressure) && finiteNonNegative(sellPressure)
	}

	beta := outcome.Fit.Beta

	if !outcome.Fit.Valid() || beta <= 0 {
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
	configured := config.DeltaT

	if interval > 0 && interval.Seconds() < configured {
		return interval.Seconds()
	}

	return configured
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
