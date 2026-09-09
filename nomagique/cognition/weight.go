package cognition

import (
	"encoding/binary"
	"math"
)

const (
	WeightSize         = 24
	DefaultDecayFactor = 0.9995 // Continuous decay per observation step
)

/*
PackedWeight is an unboxed 24-byte cognitive state.
*/
type PackedWeight struct {
	Count       uint64
	Probability float64
	WriteStep   uint64
}

/*
Encode writes the weight into a 24-byte buffer without heap allocation.
*/
func (w PackedWeight) Encode(dst []byte) {
	binary.LittleEndian.PutUint64(dst[0:8], w.Count)
	binary.LittleEndian.PutUint64(dst[8:16], math.Float64bits(w.Probability))
	binary.LittleEndian.PutUint64(dst[16:24], w.WriteStep)
}

/*
Decode reads an unboxed weight from a byte slice.
*/
func DecodeWeight(src []byte) PackedWeight {
	if len(src) < WeightSize {
		return PackedWeight{}
	}

	return PackedWeight{
		Count:       binary.LittleEndian.Uint64(src[0:8]),
		Probability: math.Float64frombits(binary.LittleEndian.Uint64(src[8:16])),
		WriteStep:   binary.LittleEndian.Uint64(src[16:24]),
	}
}

/*
Effective returns the decay-adjusted weight at currentStep.
*/
func (w PackedWeight) Effective(currentStep uint64, decayFactor float64) PackedWeight {
	if w.WriteStep >= currentStep || decayFactor <= 0 || decayFactor >= 1 {
		return w
	}

	// w_eff = w * decay^(currentStep - writeStep)
	multiplier := math.Pow(decayFactor, float64(currentStep-w.WriteStep))
	w.Count = uint64(math.Ceil(float64(w.Count) * multiplier))
	w.Probability *= multiplier

	return w
}

/*
Reinforce updates the association in place. Signed feedback contributes its
absolute mass to the denominator, and positive mass to the numerator:
(p + max(feedback, 0)) / (1 + abs(feedback)). One is the current unit of
association mass. Thus losses inhibit, larger grades adjust more, and zero
leaves the weight unchanged. Probability denotes association strength, not a
calibrated probability of profit. Callers supply feedback in consistent units.
*/
func (weight *PackedWeight) Reinforce(feedback ...float64) {
	if len(feedback) == 0 {
		weight.Probability += (1 - weight.Probability) / (float64(weight.Count) + 1)
		return
	}
	value := feedback[0]
	weight.Probability /= 1 + math.Abs(value)

	if value > 0 {
		weight.Probability += value / (1 + value)
	}
}
