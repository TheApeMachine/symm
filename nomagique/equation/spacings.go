package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Stamp is a nanosecond timestamp. Spacings subtract integer nanoseconds before
conversion to float.
*/
type Stamp struct {
	At int64
}

/*
Spacings owns consecutive timestamp differences within one delivery run. An
empty or one-observation run has no adjacent pair and emits no spacing.
*/
type Spacings struct {
	core.Base[Stamp, float64]
}

func NewSpacings() *Spacings {
	return &Spacings{}
}

func (op *Spacings) Next(
	in iter.Seq[core.Primitive[Stamp, Stamp]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		var previous int64
		seen := false

		for arriving := range in {
			at := arriving.Read().At

			if seen {
				if !yield(op.Carrier(float64(at - previous))) {
					return
				}
			}

			previous, seen = at, true
		}
	}
}
