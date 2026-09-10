package data

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
ReadoutInput is one raw observation, its quality, and the corroboration that
scales usable authority.
*/
type ReadoutInput struct {
	QualityReading
	Raw            float64
	Credibility    float64
	Supports       []float64
	Contradictions []float64
	Defined        bool
	Discrete       bool
}

/*
Readout is the usable authority and the value that authority conditions.
Discrete coordinates keep the raw observation; continuous values are weighted.
*/
type Readout struct {
	Authority float64
	Value     float64
}

/*
ReadoutOp composes credibility, corroboration and the usable raw value.
*/
type ReadoutOp struct {
	core.Base[ReadoutInput, Readout]
	authority *Authority
	bound     *equation.Bound[float64]
}

func NewReadout() *ReadoutOp {
	return &ReadoutOp{authority: NewAuthority(), bound: equation.NewBound[float64]()}
}

func (op *ReadoutOp) Next(
	in iter.Seq[core.Primitive[ReadoutInput, ReadoutInput]],
) iter.Seq[core.Primitive[Readout, Readout]] {
	return func(yield func(core.Primitive[Readout, Readout]) bool) {
		for arriving := range in {
			reading, err := op.Resolve(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

func (op *ReadoutOp) Resolve(input ReadoutInput) (Readout, error) {
	base, err := op.authority.Weight(input.QualityReading)

	if err != nil {
		return Readout{}, err
	}

	credibility, err := transport.Evaluate(op.bound, transport.Values(equation.BoundRecord[float64]{
		Value: input.Credibility,
		Lower: 0,
		Upper: 1,
	}))

	if err != nil {
		return Readout{}, err
	}

	support := 0.0

	for _, value := range input.Supports {
		support += value
	}

	contradiction := 0.0

	for _, value := range input.Contradictions {
		contradiction += value
	}

	authority := 0.0

	if input.Defined {
		weighted := base * credibility * (1 + support) / (1 + 2*contradiction)
		authority, err = transport.Evaluate(op.bound, transport.Values(equation.BoundRecord[float64]{
			Value: weighted,
			Lower: 0,
			Upper: 1,
		}))

		if err != nil {
			return Readout{}, err
		}
	}

	value := input.Raw * authority

	if input.Discrete {
		value = input.Raw
	}

	return Readout{Authority: authority, Value: value}, nil
}
