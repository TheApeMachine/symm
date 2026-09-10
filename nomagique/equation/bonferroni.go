package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
BonferroniInput is a p-value and the number of candidates it is tested among.
*/
type BonferroniInput[U core.Floating] struct {
	P          U
	Candidates U
}

/*
Bonferroni owns min(p * candidates, 1), the union bound.
*/
type Bonferroni[U core.Floating] struct {
	core.Base[BonferroniInput[U], U]
}

func NewBonferroni[U core.Floating]() *Bonferroni[U] {
	return &Bonferroni[U]{}
}

func (op *Bonferroni[U]) Next(
	in iter.Seq[core.Primitive[BonferroniInput[U], BonferroniInput[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			input := arriving.Read()
			value := input.P * input.Candidates

			if value > 1 {
				value = 1
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
