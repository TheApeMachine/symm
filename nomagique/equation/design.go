package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Design selects configured feature positions and prepends the affine intercept.
Its output is a design vector; it does not fit or predict.
*/
type Design[U core.Numeric] struct {
	core.Base[[]U, []U]
	gather *collection.Gather[U]
}

func NewDesign[U core.Numeric](indices []int) *Design[U] {
	return &Design[U]{gather: collection.NewGather[U](indices)}
}

func (op *Design[U]) Next(
	in iter.Seq[core.Primitive[[]U, []U]],
) iter.Seq[core.Primitive[[]U, []U]] {
	return func(yield func(core.Primitive[[]U, []U]) bool) {
		for gathered := range op.gather.Next(in) {
			features := gathered.Read()
			design := make([]U, 0, len(features)+1)
			design = append(design, 1)
			design = append(design, features...)

			if !yield(op.Carrier(design)) {
				return
			}
		}

		op.Error(op.gather.Error())
	}
}
