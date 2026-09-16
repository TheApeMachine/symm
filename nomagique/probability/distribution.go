package probability

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
Reading is one simplex and its named readouts.
*/
type Reading struct {
	Probabilities []float64
	Winner        int
	Confidence    float64
	Ambiguity     float64
	Sharpness     float64
}

/*
Distribution owns softmax then the winner, confidence, ambiguity, and sharpness.
*/
type Distribution struct {
	*core.PrimitiveError

	out Reading
}

func NewDistribution() *Distribution {
	return &Distribution{PrimitiveError: core.NewPrimitiveError()}
}

func (distribution *Distribution) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		softmax := NewSoftmax()
		var probabilities []float64

		for pPtr := range softmax.Next(in) {
			probabilities = append(probabilities, *(*float64)(pPtr))
		}

		if err := softmax.Error(); err != nil {
			distribution.Error(err)
			return
		}

		if len(probabilities) == 0 {
			distribution.Error(core.ErrShape)
			return
		}

		winner := 0

		for index, p := range probabilities {
			if p > probabilities[winner] {
				winner = index
			}
		}

		ambiguityValEval := NewAmbiguity()
		var ambiguityVal float64

		for out := range ambiguityValEval.Next(sequence.NewValues(probabilities...).Next(nil)) {
			ambiguityVal = *(*float64)(out)
		}

		err := ambiguityValEval.Error()

		if err != nil {
			distribution.Error(err)
			return
		}

		distribution.out = Reading{
			Probabilities: probabilities,
			Winner:        winner,
			Confidence:    probabilities[winner],
			Ambiguity:     ambiguityVal,
			Sharpness:     1 - ambiguityVal,
		}

		yield(unsafe.Pointer(&distribution.out))
	}
}
