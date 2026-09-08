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
	w.Count = uint64(float64(w.Count) * multiplier)
	w.Probability *= multiplier

	return w
}
