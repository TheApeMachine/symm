package equation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ValidPairInput is a predicted and actual observation.
*/
type ValidPairInput struct {
	Predicted float64
	Actual    float64
}

/*
ValidPair owns the supplied prediction/actual domain: nonzero values.
*/
type ValidPair struct {
	err error
	out bool
}

func NewValidPair() core.Primitive {
	return &ValidPair{}
}

func (op *ValidPair) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ValidPairInput)(arriving)
			op.out = input.Predicted != 0 && input.Actual != 0

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *ValidPair) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
