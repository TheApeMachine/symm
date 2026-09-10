package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Kish owns (sum w)² / sum(w²), the effective sample size of a weight stream.
*/
type Kish[U core.Floating] struct {
	core.Base[U, U]
	sum    U
	energy U
}

func NewKish[U core.Floating]() *Kish[U] {
	return &Kish[U]{}
}

func (op *Kish[U]) Write(value U) {
	op.Base.Write(value)
	op.sum = 0
	op.energy = 0
}

func (op *Kish[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			value := arriving.Read()
			op.sum += value
			op.energy += value * value

			if !yield(op.Carrier((op.sum * op.sum) / op.energy)) {
				return
			}
		}
	}
}
