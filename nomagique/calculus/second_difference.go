package calculus

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SecondDifferenceInput is three neighbouring ordinates.
*/
type SecondDifferenceInput struct {
	Left   float64
	Center float64
	Right  float64
}

/*
SecondDifference owns 2*center - left - right.
*/
type SecondDifference struct {
	err error
	out float64
}

func NewSecondDifference() core.Primitive {
	return &SecondDifference{}
}

func (op *SecondDifference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := *(*SecondDifferenceInput)(arriving)
			op.out = input.Center + input.Center - input.Left - input.Right

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *SecondDifference) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
