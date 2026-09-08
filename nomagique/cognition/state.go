package cognition

import (
	"encoding/binary"
	"math"
)

const StateSize = 24

/*
State is the packed, unboxed 24-byte cognitive node stored in the trie.
*/
type State struct {
	Count       uint64
	Probability float64
	WriteStep   uint64
}

func (s State) Encode(dst []byte) {
	binary.LittleEndian.PutUint64(dst[0:8], s.Count)
	binary.LittleEndian.PutUint64(dst[8:16], math.Float64bits(s.Probability))
	binary.LittleEndian.PutUint64(dst[16:24], s.WriteStep)
}

func DecodeState(src []byte) State {
	if len(src) < StateSize {
		return State{}
	}

	return State{
		Count:       binary.LittleEndian.Uint64(src[0:8]),
		Probability: math.Float64frombits(binary.LittleEndian.Uint64(src[8:16])),
		WriteStep:   binary.LittleEndian.Uint64(src[16:24]),
	}
}

/*
Aged applies continuous exponential forgetting up to currentStep.
*/
func (s State) Aged(currentStep uint64, decayFactor float64) State {
	if s.WriteStep >= currentStep || decayFactor >= 1.0 || decayFactor <= 0.0 {
		return s
	}

	elapsed := float64(currentStep - s.WriteStep)
	retention := math.Pow(decayFactor, elapsed)
	// Effective counts never truncate an observed state back to zero: a state
	// that was seen at least once remains at least one effective observation.
	// Ceiling preserves small counts that plain integer truncation would erase.
	s.Count = uint64(math.Ceil(float64(s.Count) * retention))
	s.Probability *= retention

	return s
}
