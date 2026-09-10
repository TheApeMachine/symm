package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
PolarizeInput is a signed value and the scale it is normalized against.
*/
type PolarizeInput[U core.Floating] struct {
	Value U
	Scale U
}

/*
PolarizeResult splits a signed value into nonnegative components.
*/
type PolarizeResult[U core.Floating] struct {
	Alpha           U
	Beta            U
	AlphaNormalized U
	BetaNormalized  U
	Scale           U
	Value           U
}

/*
Polarize splits a signed value into nonnegative components and normalizes
against a configured scale. The result preserves all components.
*/
type Polarize[U core.Floating] struct {
	core.Base[PolarizeInput[U], PolarizeResult[U]]
}

func NewPolarize[U core.Floating]() *Polarize[U] {
	return &Polarize[U]{}
}

func (op *Polarize[U]) Next(
	in iter.Seq[core.Primitive[PolarizeInput[U], PolarizeInput[U]]],
) iter.Seq[core.Primitive[PolarizeResult[U], PolarizeResult[U]]] {
	return func(yield func(core.Primitive[PolarizeResult[U], PolarizeResult[U]]) bool) {
		for arriving := range in {
			input := arriving.Read()
			alpha := input.Value
			beta := -input.Value

			if alpha < 0 {
				alpha = 0
			}

			if beta < 0 {
				beta = 0
			}

			result := PolarizeResult[U]{
				Alpha: alpha,
				Beta:  beta,
				Scale: input.Scale,
			}

			if input.Scale > 0 {
				result.AlphaNormalized = alpha / (alpha + input.Scale)
				result.BetaNormalized = beta / (beta + input.Scale)
			}

			result.Value = result.AlphaNormalized - result.BetaNormalized

			if !yield(op.Carrier(result)) {
				return
			}
		}
	}
}
