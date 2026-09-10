package learning

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
DirectionalTarget composes a finite nonnegative deadband and the sign of a delta.
Deadband is configuration of this target.
*/
type DirectionalTarget struct {
	core.Base[Observation, float64]
	Deadband float64
	finite   *logic.Finite[float64]
	abs      *calculus.Absolute[float64]
	sign     *calculus.Sign[float64]
}

func NewDirectionalTarget(deadband float64) *DirectionalTarget {
	return &DirectionalTarget{
		Deadband: deadband,
		finite:   logic.NewFinite[float64](),
		abs:      calculus.NewAbsolute[float64](),
		sign:     calculus.NewSign[float64](),
	}
}

func (op *DirectionalTarget) Next(
	in iter.Seq[core.Primitive[Observation, Observation]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			sample := arriving.Read()
			current, err := transport.Evaluate(op.finite, transport.Values(sample.Current))

			if err != nil {
				op.Error(err)
				return
			}

			past, err := transport.Evaluate(op.finite, transport.Values(sample.Past))

			if err != nil {
				op.Error(err)
				return
			}

			band, err := transport.Evaluate(op.finite, transport.Values(op.Deadband))

			if err != nil {
				op.Error(err)
				return
			}

			if !current || !past || !band || op.Deadband < 0 {
				op.Error(core.ErrDomain)
				return
			}

			delta := sample.Current - sample.Past
			magnitude, err := transport.Evaluate(op.abs, transport.Values(delta))

			if err != nil {
				op.Error(err)
				return
			}

			value := 0.0

			if magnitude > op.Deadband {
				signed, err := transport.Evaluate(op.sign, transport.Values(delta))

				if err != nil {
					op.Error(err)
					return
				}

				value = signed
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
