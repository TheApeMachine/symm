package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

// Previous retains one scalar run. After the first observation it yields two
// replayable primitives: previous, then current. The first observation has no
// comparison, so yields nothing. State is scoped by the surrounding keyed KV.
type Previous struct {
	*core.PrimitiveError
	prior []float64
}

func NewPrevious() core.Primitive {
	return &Previous{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Previous) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var current []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			current = append(current, *(*float64)(arriving))
		}

		if len(current) == 0 {
			return
		}

		prior := op.prior
		op.prior = current

		if len(prior) == 0 {
			return
		}

		for pointer := range data.NewValue[core.Primitive](
			data.NewValue(prior...), data.NewValue(current...),
		).Next(nil) {
			if !yield(pointer) {
				return
			}
		}
	}
}
