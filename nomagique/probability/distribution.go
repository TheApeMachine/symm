package probability

import (
	"github.com/theapemachine/symm/nomagique/types"
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
NewDistribution composes softmax, argmax, and ambiguity.
No structs, pure Value closure composition.
*/
type Distribution types.Value[[]float64, Reading]

func NewDistribution(logits ...types.Float) Distribution {
	softmax := NewSoftmax(logits...)
	argmax := NewArgmax(logits...)
	ambiguity := NewAmbiguity(logits...)

	return func(in []float64) Reading {
		probabilities := softmax(in)
		if len(probabilities) == 0 {
			return Reading{}
		}

		best := argmax(probabilities)
		amb := ambiguity(probabilities)

		return Reading{
			Probabilities: probabilities,
			Winner:        best.Index,
			Confidence:    best.Value,
			Ambiguity:     amb,
			Sharpness:     1 - amb,
		}
	}
}
