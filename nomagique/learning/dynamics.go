package learning

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

const PredictiveDynamicsKey = "predictive_dynamics"

var (
	SymbolDynamicsTime               = types.MustIntern("predictive/time")
	SymbolDynamicsPosition           = types.MustIntern("predictive/position")
	SymbolDynamicsActivity           = types.MustIntern("predictive/activity")
	SymbolDynamicsExternalPower      = types.MustIntern("predictive/external_power")
	SymbolDynamicsPhase              = types.MustIntern("predictive/phase")
	SymbolDynamicsReady              = types.MustIntern("predictive/ready")
	SymbolDynamicsDeltaTime          = types.MustIntern("predictive/delta_time")
	SymbolDynamicsVelocity           = types.MustIntern("predictive/velocity")
	SymbolDynamicsAcceleration       = types.MustIntern("predictive/acceleration")
	SymbolDynamicsMemory             = types.MustIntern("predictive/memory")
	SymbolDynamicsMemoryScale        = types.MustIntern("predictive/memory_scale")
	SymbolDynamicsStoredEnergy       = types.MustIntern("predictive/stored_energy")
	SymbolDynamicsSuppliedPower      = types.MustIntern("predictive/supplied_power")
	SymbolDynamicsDissipation        = types.MustIntern("predictive/dissipation")
	SymbolDynamicsPassivityResidue   = types.MustIntern("predictive/passivity_residue")
	SymbolDynamicsContinuousMean     = types.MustIntern("predictive/continuous_mean")
	SymbolDynamicsContinuousM2       = types.MustIntern("predictive/continuous_m2")
	SymbolDynamicsContinuousVariance = types.MustIntern("predictive/continuous_variance")
	SymbolDynamicsJumpAmplitude      = types.MustIntern("predictive/jump_amplitude")
	SymbolDynamicsJumpMean           = types.MustIntern("predictive/jump_mean")
	SymbolDynamicsJumpM2             = types.MustIntern("predictive/jump_m2")
	SymbolDynamicsJumpVariance       = types.MustIntern("predictive/jump_variance")
	SymbolDynamicsSampleCount        = types.MustIntern("predictive/sample_count")
	SymbolDynamicsRotorScalar        = types.MustIntern("predictive/rotor_scalar")
	SymbolDynamicsRotorBivector      = types.MustIntern("predictive/rotor_bivector")
	SymbolDynamicsEquivarianceNorm   = types.MustIntern("predictive/equivariance_norm")
	// Previous slots carry the last committed observation. The loader writes
	// the current observation into the unprefixed predictive slots; the
	// primitive rolls them into the previous slots so both are visible in one
	// frame without collision.
	SymbolDynamicsPreviousTime     = types.MustIntern("predictive/previous_time")
	SymbolDynamicsPreviousPosition = types.MustIntern("predictive/previous_position")
	SymbolDynamicsPreviousActivity = types.MustIntern("predictive/previous_activity")
	SymbolDynamicsPreviousVelocity = types.MustIntern("predictive/previous_velocity")
	SymbolDynamicsPreviousMemory   = types.MustIntern("predictive/previous_memory")
	SymbolDynamicsPreviousEnergy   = types.MustIntern("predictive/previous_energy")
)

/*
PredictiveDynamics augments a scalar latent trajectory with continuous-time
motion, liquid memory, port-Hamiltonian accounting, Hawkes-compatible jump
separation, and a unit phase rotor. The loader feeds the current observation
(time, position, activity, external power, optional phase); the primitive reads
the previous observation from the previous slots, computes, and rolls the
current observation into the previous slots for the next step. Every retained
value is carried in Frame slots, so keyed streams can own the primitive without
hidden mutable fields.
*/
func PredictiveDynamics(input *types.Frame) {
	observedAt, hasObservedAt := input.Get(SymbolDynamicsTime)
	position, hasPosition := input.Get(SymbolDynamicsPosition)

	if !hasObservedAt || !hasPosition {
		input.Err = fmt.Errorf(
			"predictive dynamics: time and position required",
		)
		return
	}

	activity, _ := input.Get(SymbolDynamicsActivity)
	externalPower, _ := input.Get(SymbolDynamicsExternalPower)
	phase, hasPhase := input.Get(SymbolDynamicsPhase)
	previousAt, initialized := input.Get(SymbolDynamicsPreviousTime)

	if !initialized {
		initializePredictiveDynamics(
			input,
			observedAt,
			position,
			activity,
			phase,
			hasPhase,
		)
		return
	}

	if observedAt < previousAt {
		input.Err = fmt.Errorf(
			"predictive dynamics: event time must not regress",
		)
		return
	}

	if observedAt == previousAt {
		return
	}

	deltaTime := observedAt - previousAt
	previousPosition, hasPreviousPosition := input.Get(SymbolDynamicsPreviousPosition)

	if !hasPreviousPosition {
		input.Err = fmt.Errorf(
			"predictive dynamics: previous position required",
		)
		return
	}

	previousVelocity, _ := input.Get(SymbolDynamicsPreviousVelocity)
	previousMemory, _ := input.Get(SymbolDynamicsPreviousMemory)
	previousEnergy, _ := input.Get(SymbolDynamicsPreviousEnergy)
	previousActivity, _ := input.Get(SymbolDynamicsPreviousActivity)
	positionDelta := position - previousPosition
	velocity := positionDelta / deltaTime
	acceleration := (velocity - previousVelocity) / deltaTime
	activityMagnitude := math.Abs(activity)
	memoryScale := deltaTime * (1 + 1/(1+activityMagnitude))
	memoryDecay := math.Exp(-deltaTime / memoryScale)
	memory := memoryDecay*previousMemory + (1-memoryDecay)*position
	damping := 1 / memoryScale
	storedEnergy := 0.5 * (position*position + velocity*velocity)
	suppliedPower := externalPower * velocity
	dissipation := damping * velocity * velocity
	energyRate := (storedEnergy - previousEnergy) / deltaTime
	passivityResidue := energyRate - suppliedPower + dissipation
	activityDelta := activity - previousActivity
	jumpAmplitude := activityDelta * positionDelta / (1 + math.Abs(activityDelta))
	continuousIncrement := positionDelta - jumpAmplitude
	continuousRate := continuousIncrement / math.Sqrt(deltaTime)
	sampleCountValue, _ := input.Get(SymbolDynamicsSampleCount)
	sampleCount := sampleCountValue + 1
	continuousMean, continuousM2 := updateMoments(
		input,
		SymbolDynamicsContinuousMean,
		SymbolDynamicsContinuousM2,
		continuousRate,
		sampleCount,
	)
	jumpMean, jumpM2 := updateMoments(
		input,
		SymbolDynamicsJumpMean,
		SymbolDynamicsJumpM2,
		jumpAmplitude,
		sampleCount,
	)
	continuousVariance := sampleVariance(continuousM2, sampleCount)
	jumpVariance := sampleVariance(jumpM2, sampleCount)

	if !hasPhase {
		phase = math.Atan2(velocity, position)
	}

	rotorScalar := math.Cos(phase / 2)
	rotorBivector := math.Sin(phase / 2)
	equivarianceNorm := rotorScalar*rotorScalar + rotorBivector*rotorBivector
	input.Put(SymbolDynamicsPreviousTime, observedAt)
	input.Put(SymbolDynamicsPreviousPosition, position)
	input.Put(SymbolDynamicsPreviousActivity, activity)
	input.Put(SymbolDynamicsPreviousVelocity, velocity)
	input.Put(SymbolDynamicsPreviousMemory, memory)
	input.Put(SymbolDynamicsPreviousEnergy, storedEnergy)
	input.Put(SymbolDynamicsReady, 1)
	input.Put(SymbolDynamicsDeltaTime, deltaTime)
	input.Put(SymbolDynamicsVelocity, velocity)
	input.Put(SymbolDynamicsAcceleration, acceleration)
	input.Put(SymbolDynamicsMemory, memory)
	input.Put(SymbolDynamicsMemoryScale, memoryScale)
	input.Put(SymbolDynamicsStoredEnergy, storedEnergy)
	input.Put(SymbolDynamicsSuppliedPower, suppliedPower)
	input.Put(SymbolDynamicsDissipation, dissipation)
	input.Put(SymbolDynamicsPassivityResidue, passivityResidue)
	input.Put(SymbolDynamicsContinuousMean, continuousMean)
	input.Put(SymbolDynamicsContinuousM2, continuousM2)
	input.Put(SymbolDynamicsContinuousVariance, continuousVariance)
	input.Put(SymbolDynamicsJumpAmplitude, jumpAmplitude)
	input.Put(SymbolDynamicsJumpMean, jumpMean)
	input.Put(SymbolDynamicsJumpM2, jumpM2)
	input.Put(SymbolDynamicsJumpVariance, jumpVariance)
	input.Put(SymbolDynamicsSampleCount, sampleCount)
	input.Put(SymbolDynamicsRotorScalar, rotorScalar)
	input.Put(SymbolDynamicsRotorBivector, rotorBivector)
	input.Put(SymbolDynamicsEquivarianceNorm, equivarianceNorm)
}

func initializePredictiveDynamics(
	input *types.Frame,
	observedAt float64,
	position float64,
	activity float64,
	phase float64,
	hasPhase bool,
) {
	if !hasPhase {
		phase = 0
	}

	rotorScalar := math.Cos(phase / 2)
	rotorBivector := math.Sin(phase / 2)
	storedEnergy := 0.5 * position * position
	input.Put(SymbolDynamicsPreviousTime, observedAt)
	input.Put(SymbolDynamicsPreviousPosition, position)
	input.Put(SymbolDynamicsPreviousActivity, activity)
	input.Put(SymbolDynamicsPreviousVelocity, 0)
	input.Put(SymbolDynamicsPreviousMemory, position)
	input.Put(SymbolDynamicsPreviousEnergy, storedEnergy)
	input.Put(SymbolDynamicsReady, 0)
	input.Put(SymbolDynamicsMemory, position)
	input.Put(SymbolDynamicsStoredEnergy, storedEnergy)
	input.Put(SymbolDynamicsSampleCount, 0)
	input.Put(SymbolDynamicsRotorScalar, rotorScalar)
	input.Put(SymbolDynamicsRotorBivector, rotorBivector)
	input.Put(
		SymbolDynamicsEquivarianceNorm,
		rotorScalar*rotorScalar+rotorBivector*rotorBivector,
	)
}

func updateMoments(
	input *types.Frame,
	meanSymbol types.Symbol,
	m2Symbol types.Symbol,
	sample float64,
	count float64,
) (float64, float64) {
	previousMean, _ := input.Get(meanSymbol)
	previousM2, _ := input.Get(m2Symbol)
	delta := sample - previousMean
	mean := previousMean + delta/count
	m2 := previousM2 + delta*(sample-mean)

	return mean, m2
}

func sampleVariance(m2 float64, count float64) float64 {
	if count < 2 {
		return 0
	}

	return m2 / (count - 1)
}
