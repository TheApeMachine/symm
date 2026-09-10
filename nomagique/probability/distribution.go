package probability

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Reading is one simplex and its named readouts. There is no second Collection
interface and no accessors that bypass Primitive delivery.
*/
type Reading struct {
	Probabilities []float64
	Winner        int
	Confidence    float64
	Ambiguity     float64
	Sharpness     float64
}

/*
Distribution owns softmax then the winner, confidence, and ambiguity of that
simplex.
*/
type Distribution struct {
	core.Base[float64, Reading]
}

func NewDistribution() *Distribution {
	return &Distribution{}
}

func (op *Distribution) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[Reading, Reading]] {
	return func(yield func(core.Primitive[Reading, Reading]) bool) {
		softmax := equation.NewSoftmax[float64]()
		var probabilities []float64

		for probability := range softmax.Next(in) {
			probabilities = append(probabilities, probability.Read())
		}

		if err := softmax.Error(); err != nil {
			op.Error(err)
			return
		}

		winner, err := transport.Evaluate(equation.NewArgmax[float64](), transport.Values(probabilities...))

		if err != nil {
			op.Error(err)
			return
		}

		ambiguity := 0.0

		for out := range NewAmbiguity().Next(transport.Values(probabilities...)) {
			ambiguity = out.Read()
		}

		if !yield(op.Carrier(Reading{
			Probabilities: probabilities,
			Winner:        winner.Index,
			Confidence:    winner.Value,
			Ambiguity:     ambiguity,
			Sharpness:     1 - ambiguity,
		})) {
			return
		}
	}
}
