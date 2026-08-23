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
	SymbolDynamicsPreviousActivity   = types.MustIntern("predictive/previous_activity")
	SymbolDynamicsRotorScalar        = types.MustIntern("predictive/rotor_scalar")
	SymbolDynamicsRotorBivector      = types.MustIntern("predictive/rotor_bivector")
	SymbolDynamicsEquivarianceNorm   = types.MustIntern("predictive/equivariance_norm")
)

/*
PredictiveDynamics augments a scalar latent trajectory with continuous-time
motion, liquid memory, port-Hamiltonian accounting, Hawkes-compatible jump
separation, and a unit phase rotor. Every retained value is carried in Frame
state, so keyed streams can own the primitive without hidden mutable fields.
*/
func PredictiveDynamics(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	observedAt, hasObservedAt := input.Get(SymbolDynamicsTime)
	position, hasPosition := input.Get(SymbolDynamicsPosition)

	if !hasObservedAt || !hasPosition {
		return state, types.Frame{}, fmt.Errorf(
			"predictive dynamics: time and position required",
		)
	}

	activity, _ := input.Get(SymbolDynamicsActivity)
	externalPower, _ := input.Get(SymbolDynamicsExternalPower)
	phase, hasPhase := input.Get(SymbolDynamicsPhase)
	previousAt, initialized := state.Get(SymbolDynamicsTime)

	if !initialized {
		return initializePredictiveDynamics(
			state,
			observedAt,
			position,
			activity,
			phase,
			hasPhase,
		)
	}

	if observedAt < previousAt {
		return state, types.Frame{}, fmt.Errorf(
			"predictive dynamics: event time must not regress",
		)
	}

	if observedAt == previousAt {
		return state, predictiveDynamicsOutput(state), nil
	}

	deltaTime := observedAt - previousAt
	previousPosition := state.MustGet(SymbolDynamicsPosition)
	previousVelocity, _ := state.Get(SymbolDynamicsVelocity)
	previousMemory, _ := state.Get(SymbolDynamicsMemory)
	previousEnergy, _ := state.Get(SymbolDynamicsStoredEnergy)
	previousActivity, _ := state.Get(SymbolDynamicsPreviousActivity)
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
	sampleCountValue, _ := state.Get(SymbolDynamicsSampleCount)
	sampleCount := sampleCountValue + 1
	continuousMean, continuousM2 := updateMoments(
		state,
		SymbolDynamicsContinuousMean,
		SymbolDynamicsContinuousM2,
		continuousRate,
		sampleCount,
	)
	jumpMean, jumpM2 := updateMoments(
		state,
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
	nextState := state
	nextState.Put(SymbolDynamicsTime, observedAt)
	nextState.Put(SymbolDynamicsPosition, position)
	nextState.Put(SymbolDynamicsPreviousActivity, activity)
	nextState.Put(SymbolDynamicsReady, 1)
	nextState.Put(SymbolDynamicsDeltaTime, deltaTime)
	nextState.Put(SymbolDynamicsVelocity, velocity)
	nextState.Put(SymbolDynamicsAcceleration, acceleration)
	nextState.Put(SymbolDynamicsMemory, memory)
	nextState.Put(SymbolDynamicsMemoryScale, memoryScale)
	nextState.Put(SymbolDynamicsStoredEnergy, storedEnergy)
	nextState.Put(SymbolDynamicsSuppliedPower, suppliedPower)
	nextState.Put(SymbolDynamicsDissipation, dissipation)
	nextState.Put(SymbolDynamicsPassivityResidue, passivityResidue)
	nextState.Put(SymbolDynamicsContinuousMean, continuousMean)
	nextState.Put(SymbolDynamicsContinuousM2, continuousM2)
	nextState.Put(SymbolDynamicsContinuousVariance, continuousVariance)
	nextState.Put(SymbolDynamicsJumpAmplitude, jumpAmplitude)
	nextState.Put(SymbolDynamicsJumpMean, jumpMean)
	nextState.Put(SymbolDynamicsJumpM2, jumpM2)
	nextState.Put(SymbolDynamicsJumpVariance, jumpVariance)
	nextState.Put(SymbolDynamicsSampleCount, sampleCount)
	nextState.Put(SymbolDynamicsRotorScalar, rotorScalar)
	nextState.Put(SymbolDynamicsRotorBivector, rotorBivector)
	nextState.Put(SymbolDynamicsEquivarianceNorm, equivarianceNorm)

	return nextState, predictiveDynamicsOutput(nextState), nil
}

func initializePredictiveDynamics(
	state types.Frame,
	observedAt float64,
	position float64,
	activity float64,
	phase float64,
	hasPhase bool,
) (types.Frame, types.Frame, error) {
	if !hasPhase {
		phase = 0
	}

	rotorScalar := math.Cos(phase / 2)
	rotorBivector := math.Sin(phase / 2)
	storedEnergy := 0.5 * position * position
	nextState := state
	nextState.Put(SymbolDynamicsTime, observedAt)
	nextState.Put(SymbolDynamicsPosition, position)
	nextState.Put(SymbolDynamicsPreviousActivity, activity)
	nextState.Put(SymbolDynamicsReady, 0)
	nextState.Put(SymbolDynamicsMemory, position)
	nextState.Put(SymbolDynamicsStoredEnergy, storedEnergy)
	nextState.Put(SymbolDynamicsSampleCount, 0)
	nextState.Put(SymbolDynamicsRotorScalar, rotorScalar)
	nextState.Put(SymbolDynamicsRotorBivector, rotorBivector)
	nextState.Put(
		SymbolDynamicsEquivarianceNorm,
		rotorScalar*rotorScalar+rotorBivector*rotorBivector,
	)

	return nextState, predictiveDynamicsOutput(nextState), nil
}

func updateMoments(
	state types.Frame,
	meanSymbol types.Symbol,
	m2Symbol types.Symbol,
	sample float64,
	count float64,
) (float64, float64) {
	previousMean, _ := state.Get(meanSymbol)
	previousM2, _ := state.Get(m2Symbol)
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

func predictiveDynamicsOutput(state types.Frame) types.Frame {
	output := types.Frame{}

	for _, symbol := range []types.Symbol{
		SymbolDynamicsReady,
		SymbolDynamicsDeltaTime,
		SymbolDynamicsPosition,
		SymbolDynamicsVelocity,
		SymbolDynamicsAcceleration,
		SymbolDynamicsMemory,
		SymbolDynamicsMemoryScale,
		SymbolDynamicsStoredEnergy,
		SymbolDynamicsSuppliedPower,
		SymbolDynamicsDissipation,
		SymbolDynamicsPassivityResidue,
		SymbolDynamicsContinuousVariance,
		SymbolDynamicsJumpAmplitude,
		SymbolDynamicsJumpVariance,
		SymbolDynamicsSampleCount,
		SymbolDynamicsRotorScalar,
		SymbolDynamicsRotorBivector,
		SymbolDynamicsEquivarianceNorm,
	} {
		value, found := state.Get(symbol)

		if found {
			output.Put(symbol, value)
		}
	}

	return output
}
