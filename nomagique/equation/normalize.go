package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Normalize divides each arrival by the run's total. A collection is not complete
until its run is spent, so nothing is handed over until everything has arrived.
Zero totals remain mathematically undefined.
*/
type Normalize[U core.Floating] struct {
	core.Base[U, U]
}

func NewNormalize[U core.Floating]() *Normalize[U] {
	return &Normalize[U]{}
}

func (op *Normalize[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		var values []U
		var total U

		for arriving := range in {
			value := arriving.Read()
			values = append(values, value)
			total += value
		}

		for _, value := range values {
			if !yield(op.Carrier(value / total)) {
				return
			}
		}
	}
}
