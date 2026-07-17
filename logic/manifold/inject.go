package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

const speedLimitTolerance = 1e-9

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
velocity keeps the deposited gas state conservative. When the raw impulse would
outrun the event-time Courant bound, the increments are scaled uniformly so
direction and buy/sell ratio survive while the gas step stays finite.
*/
func applyForcing(
	config pmanifold.Config,
	outcome excitation.Outcome,
	interval time.Duration,
	oscillators []pmanifold.Oscillator,
) (characteristicSpeed float64, scale float64, err error) {
	deltaT := integrationDeltaT(config, interval)
	buyPressure, sellPressure, ready := arrivalForcing(outcome, deltaT)

	if !ready {
		return 0, 0, errnie.Err(
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
		return 0, 0, errnie.Err(
			errnie.Validation,
			"manifold: L3 carriers require both book sides for forcing",
			nil,
		)
	}

	bases := make([]float64, len(oscillators))
	increments := make([]float64, len(oscillators))
	scale = 1

	for index := range oscillators {
		pressure, mass := buyPressure, buyMass

		if oscillators[index].PosX < midpoint {
			pressure, mass = -sellPressure, sellMass
		}

		bases[index] = oscillators[index].VelX
		increments[index] = pressure / mass
		oscillators[index].VelX = bases[index] + increments[index]
	}

	characteristicSpeed, err = boundSpeed(config, oscillators)

	if err != nil {
		return 0, 0, err
	}

	speedLimit := config.AdvectiveDeltaT(1) / deltaT

	if speedLimit > 0 && characteristicSpeed > speedLimit {
		characteristicSpeed, scale, err = rescaleImpulse(
			config,
			oscillators,
			bases,
			increments,
			characteristicSpeed,
			speedLimit,
		)

		if err != nil {
			return 0, 0, err
		}

		if scale > 0 && !withinSpeedLimit(characteristicSpeed, speedLimit) {
			return 0, 0, errnie.Err(
				errnie.Validation,
				"manifold: forced carrier speed exceeds Courant bound",
				nil,
			)
		}
	}

	if characteristicSpeed <= 0 {
		return 0, 0, errnie.Err(
			errnie.Validation,
			"manifold: carrier characteristic speed must be positive",
			nil,
		)
	}

	return characteristicSpeed, scale, nil
}

/*
rescaleImpulse shrinks the Hawkes velocity increments uniformly until the
population rarefaction fits the event-time Courant bound. Prior carrier motion
is left intact when it alone already saturates that bound.
*/
func rescaleImpulse(
	config pmanifold.Config,
	oscillators []pmanifold.Oscillator,
	bases []float64,
	increments []float64,
	fullSpeed float64,
	speedLimit float64,
) (characteristicSpeed float64, scale float64, err error) {
	for index := range oscillators {
		oscillators[index].VelX = bases[index]
	}

	baseSpeed, err := boundSpeed(config, oscillators)

	if err != nil {
		return 0, 0, err
	}

	if baseSpeed >= speedLimit {
		return baseSpeed, 0, nil
	}

	scale = (speedLimit - baseSpeed) / (fullSpeed - baseSpeed)

	if !finiteNonNegative(scale) || scale > 1 {
		scale = 1
	}

	for index := range oscillators {
		oscillators[index].VelX = bases[index] + scale*increments[index]
	}

	characteristicSpeed, err = boundSpeed(config, oscillators)

	if err != nil {
		return 0, 0, err
	}

	if characteristicSpeed <= speedLimit {
		if withinSpeedLimit(characteristicSpeed, speedLimit) {
			return characteristicSpeed, scale, nil
		}
	}

	scale *= speedLimit / characteristicSpeed

	for index := range oscillators {
		oscillators[index].VelX = bases[index] + scale*increments[index]
	}

	characteristicSpeed, err = boundSpeed(config, oscillators)

	if err != nil {
		return 0, 0, err
	}

	if withinSpeedLimit(characteristicSpeed, speedLimit) {
		return characteristicSpeed, scale, nil
	}

	return bisectImpulseScale(
		config,
		oscillators,
		bases,
		increments,
		speedLimit,
		scale,
	)
}

/*
bisectImpulseScale shrinks a uniform impulse scale until boundSpeed respects
the event-time Courant limit within speedLimitTolerance.
*/
func bisectImpulseScale(
	config pmanifold.Config,
	oscillators []pmanifold.Oscillator,
	bases []float64,
	increments []float64,
	speedLimit float64,
	upperScale float64,
) (characteristicSpeed float64, scale float64, err error) {
	low := 0.0
	high := upperScale

	if high <= 0 {
		high = 1
	}

	for range 64 {
		mid := (low + high) / 2

		for index := range oscillators {
			oscillators[index].VelX = bases[index] + mid*increments[index]
		}

		speed, boundErr := boundSpeed(config, oscillators)

		if boundErr != nil {
			return 0, 0, boundErr
		}

		if speed > speedLimit {
			high = mid
			continue
		}

		low = mid
		characteristicSpeed = speed
		scale = mid
	}

	if !withinSpeedLimit(characteristicSpeed, speedLimit) {
		return 0, 0, errnie.Err(
			errnie.Validation,
			"manifold: rescaled impulse still exceeds Courant bound",
			nil,
		)
	}

	return characteristicSpeed, scale, nil
}

/*
withinSpeedLimit reports whether a rarefaction speed respects the Courant cap
within floating-point tolerance.
*/
func withinSpeedLimit(speed float64, speedLimit float64) bool {
	return speed <= speedLimit*(1+speedLimitTolerance)
}

/*
boundSpeed is the stricter of the rarefaction head and the HLL Courant speed.
*/
func boundSpeed(
	config pmanifold.Config,
	oscillators []pmanifold.Oscillator,
) (float64, error) {
	rarefaction, err := rarefactionSpeed(config, oscillators)

	if err != nil {
		return 0, err
	}

	courant, err := gasCourantSpeed(config, oscillators)

	if err != nil {
		return 0, err
	}

	if courant > rarefaction {
		return courant, nil
	}

	return rarefaction, nil
}

/*
rarefactionSpeed returns the fastest 1D rarefaction head after forcing.
*/
func rarefactionSpeed(
	config pmanifold.Config,
	oscillators []pmanifold.Oscillator,
) (float64, error) {
	return carrierSpeed(config, oscillators, true)
}

/*
gasCourantSpeed returns the fastest multidimensional HLL Courant speed (|u|+c)
matching the Metal gas_rhs_cell stability check.
*/
func gasCourantSpeed(
	config pmanifold.Config,
	oscillators []pmanifold.Oscillator,
) (float64, error) {
	return carrierSpeed(config, oscillators, false)
}

/*
carrierSpeed evaluates either the 1D rarefaction head or the HLL |u|+c speed
from forced carrier thermodynamics.
*/
func carrierSpeed(
	config pmanifold.Config,
	oscillators []pmanifold.Oscillator,
	rarefaction bool,
) (float64, error) {
	characteristicSpeed := 0.0

	for _, oscillator := range oscillators {
		if oscillator.Amplitude <= 0 {
			return 0, errnie.Err(
				errnie.Validation,
				"manifold: carrier amplitude must be positive",
				nil,
			)
		}

		velocity := math.Sqrt(
			oscillator.VelX*oscillator.VelX +
				oscillator.VelY*oscillator.VelY +
				oscillator.VelZ*oscillator.VelZ,
		)
		specificInternalEnergy := oscillator.Heat / oscillator.Amplitude

		if !finiteNonNegative(specificInternalEnergy) {
			return 0, errnie.Err(
				errnie.Validation,
				"manifold: carrier specific internal energy is not finite",
				nil,
			)
		}

		soundSpeed := math.Sqrt(
			config.Gamma * (config.Gamma - 1) * specificInternalEnergy,
		)
		speed := velocity + soundSpeed

		if rarefaction {
			speed = velocity + 2*soundSpeed/(config.Gamma-1)
		}

		if !finiteNonNegative(speed) {
			return 0, errnie.Err(
				errnie.Validation,
				"manifold: carrier characteristic speed is not finite",
				nil,
			)
		}

		if speed > characteristicSpeed {
			characteristicSpeed = speed
		}
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
