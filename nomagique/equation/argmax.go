package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ArgmaxResult is a winning value and the first index at which it occurred.
*/
type ArgmaxResult[U core.Numeric] struct {
	Index int
	Value U
}

/*
Argmax preserves a winning value's ordinal through comparison. Strict
comparison keeps the first equal maximum.
*/
type Argmax[U core.Numeric] struct {
	core.Base[U, ArgmaxResult[U]]
}

func NewArgmax[U core.Numeric]() *Argmax[U] {
	return &Argmax[U]{}
}

func (op *Argmax[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[ArgmaxResult[U], ArgmaxResult[U]]] {
	return func(yield func(core.Primitive[ArgmaxResult[U], ArgmaxResult[U]]) bool) {
		var best ArgmaxResult[U]
		seen := false
		index := 0

		for arriving := range in {
			value := arriving.Read()

			if !seen || value > best.Value {
				best = ArgmaxResult[U]{Index: index, Value: value}
				seen = true
			}

			index++
		}

		if !seen {
			return
		}

		if !yield(op.Carrier(best)) {
			return
		}
	}
}
