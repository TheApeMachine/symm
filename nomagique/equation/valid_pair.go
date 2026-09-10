package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ValidPairInput is a predicted and actual observation.
*/
type ValidPairInput[U core.Floating] struct {
	Predicted U
	Actual    U
}

/*
ValidPair owns the supplied prediction/actual domain: finite, nonzero values.
*/
type ValidPair[U core.Floating] struct {
	core.Base[ValidPairInput[U], bool]
}

func NewValidPair[U core.Floating]() *ValidPair[U] {
	return &ValidPair[U]{}
}

func (op *ValidPair[U]) Next(
	in iter.Seq[core.Primitive[ValidPairInput[U], ValidPairInput[U]]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			input := arriving.Read()
			predicted := float64(input.Predicted)
			actual := float64(input.Actual)
			ok := !math.IsNaN(predicted) && !math.IsInf(predicted, 0) && predicted != 0 &&
				!math.IsNaN(actual) && !math.IsInf(actual, 0) && actual != 0

			if !yield(op.Carrier(ok)) {
				return
			}
		}
	}
}
