package adaptive

import (
	"math"
)

/*
ClockType defines the information-time clock strategy.
*/
type ClockType int

const (
	INTERARRIVAL ClockType = iota
	VOLUME
	ENTROPY
)

/*
SensitivityType defines the responsiveness of the adaptive clock.
*/
type SensitivityType int

const (
	HIGH SensitivityType = iota
	MEDIUM
	LOW
)

// Sensitivity multipliers for information-time clock pacing.
// HighSensitivityMultiplier (2.0): accelerates clock pacing under burst conditions.
// MediumSensitivityMultiplier (1.0): identity natural pacing.
// LowSensitivityMultiplier (0.5): sublinear damped pacing.
const (
	HighSensitivityMultiplier   = 2.0
	MediumSensitivityMultiplier = 1.0
	LowSensitivityMultiplier    = 0.5
)

// EntropyBinCount is the discrete quantization resolution for entropy estimation (8 bins).
const EntropyBinCount = 8

// MaximumShannonEntropyForEightBins = log2(8) = 3.0 bits.
const MaximumShannonEntropyForEightBins = 3.0

/*
Sensitivity configures clock responsiveness without magic constants.
*/
type Sensitivity struct {
	Type SensitivityType
}

/*
Clock decays in information time (tick volume, arrival entropy, interarrival variance)
rather than frozen calendar milliseconds.
Implements Step(Number) Number (Node contract).
*/
type Clock struct {
	Type        ClockType
	Sensitivity Sensitivity

	count      float64
	welford    WelfordEngine
	lastSample float64
	entropy    float64
	bins       [EntropyBinCount]float64
}

// Step calculates the emergent information-time elapsed increment.
func (clock *Clock) Step(number Number) Number {
	value := float64(number)
	clock.count++
	factor := MediumSensitivityMultiplier

	switch clock.Sensitivity.Type {
	case HIGH:
		factor = HighSensitivityMultiplier
	case MEDIUM:
		factor = MediumSensitivityMultiplier
	case LOW:
		factor = LowSensitivityMultiplier
	default:
		factor = MediumSensitivityMultiplier
	}

	switch clock.Type {
	case INTERARRIVAL:
		// Canonical renewal time: delta_tau = delta_t / E[delta_t]
		mean, _ := clock.welford.Update(value)

		if mean <= 0 {
			return Number(factor)
		}

		// Normalized event interval relative to expected arrival rate
		return Number((math.Abs(value) / mean) * factor)

	case VOLUME:
		// Canonical volume time: delta_tau = volume / E[volume]
		magnitude := math.Abs(value)
		mean, _ := clock.welford.Update(magnitude)

		if mean <= 0 {
			return Number(factor)
		}

		return Number((magnitude / mean) * factor)

	case ENTROPY:
		// Shannon entropy over quantized arrival bins: delta_tau = (H / H_max) * factor
		binIndex := int(math.Mod(math.Abs(value), float64(EntropyBinCount)))
		clock.bins[binIndex]++

		var total float64

		for _, binCount := range clock.bins {
			total += binCount
		}

		var entropy float64

		if total > 0 {
			for _, binCount := range clock.bins {
				if binCount > 0 {
					probability := binCount / total
					entropy -= probability * math.Log2(probability)
				}
			}
		}

		clock.entropy = entropy
		normalizedEntropy := entropy / MaximumShannonEntropyForEightBins

		return Number(normalizedEntropy * factor)

	default:
		return Number(factor)
	}
}
