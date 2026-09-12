package data

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
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
	err       error
	authority core.Primitive
	bound     core.Primitive
	out       Readout
}

func NewReadout() core.Primitive {
	return &ReadoutOp{
		authority: NewAuthority(),
		bound:     calculus.NewBound(),
	}
}

func (op *ReadoutOp) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ReadoutInput)(arriving)

			var base float64
			for out := range op.authority.Next(transport.NewValues(input.QualityReading).Next(nil)) {
				base = *(*float64)(out)
			}

			if err := op.authority.Error(); err != nil {
				op.Error(err)
				return
			}

			credRecord := calculus.BoundRecord{
				Value: input.Credibility,
				Lower: 0,
				Upper: 1,
			}
			var credibility float64
			for out := range op.bound.Next(transport.NewValues(credRecord).Next(nil)) {
				credibility = *(*float64)(out)
			}

			if err := op.bound.Error(); err != nil {
				op.Error(err)
				return
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
				boundRec := calculus.BoundRecord{
					Value: weighted,
					Lower: 0,
					Upper: 1,
				}
				for out := range op.bound.Next(transport.NewValues(boundRec).Next(nil)) {
					authority = *(*float64)(out)
				}

				if err := op.bound.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			value := input.Raw * authority

			if input.Discrete {
				value = input.Raw
			}

			op.out = Readout{
				Authority: authority,
				Value:     value,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *ReadoutOp) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
