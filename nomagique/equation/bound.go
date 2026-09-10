package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
BoundRecord is a value and the interval that may replace it.
*/
type BoundRecord[U core.Numeric] struct {
	Value U
	Lower U
	Upper U
}

/*
Bound selects lower, value, or upper. A NaN value compares as unordered and
passes through rather than becoming a bound.
*/
type Bound[U core.Numeric] struct {
	core.Base[BoundRecord[U], U]
}

func NewBound[U core.Numeric]() *Bound[U] {
	return &Bound[U]{}
}

func (op *Bound[U]) Next(
	in iter.Seq[core.Primitive[BoundRecord[U], BoundRecord[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			record := arriving.Read()
			value := record.Value

			if value < record.Lower {
				value = record.Lower
			}

			if value > record.Upper {
				value = record.Upper
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
