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
	*core.PrimitiveError

	out bool
}

func NewValidPair() *ValidPair {
	return &ValidPair{PrimitiveError: core.NewPrimitiveError()}
}

func (validPair *ValidPair) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ValidPairInput)(arriving)
			validPair.out = input.Predicted != 0 && input.Actual != 0

			if !yield(unsafe.Pointer(&validPair.out)) {
				return
			}
		}
	}
}
