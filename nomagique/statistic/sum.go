package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

// Sum retains an arithmetic total across Next calls. Each non-empty run
// contributes its scalar arrivals and yields exactly one updated total.
// A malformed run does not commit a partial update.
type Sum struct {
	*core.PrimitiveError
	total float64
}

func NewSum() core.Primitive {
	return &Sum{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Sum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil || in == nil {
			return
		}

		total := op.total
		observed := false

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			total += *(*float64)(arriving)
			observed = true
		}

		if !observed {
			return
		}

		op.total = total

		for value := range data.NewValue(total).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
