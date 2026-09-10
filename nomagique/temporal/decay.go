package temporal

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Decay multiplies each arrival by a retention factor. The clock yields elapsed
time; the shape yields the factor for that elapsed time. A missing clock is
infinite elapsed time. A missing shape is linear retention, floored at zero.
*/
type Decay[U core.Floating] struct {
	core.Base[U, U]
	clock  core.Primitive[U, U]
	shape  core.Primitive[U, U]
	linear bool
}

func NewDecay[U core.Floating](clock, shape core.Primitive[U, U]) *Decay[U] {
	linear := shape == nil

	if clock == nil {
		clock = store.NewConstant[U, U](U(math.Inf(1)))
	}

	if shape == nil {
		shape = calculus.NewMaximum[U](0)
	}

	return &Decay[U]{clock: clock, shape: shape, linear: linear}
}

func (op *Decay[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			value := arriving.Read()
			elapsed := U(0)

			for tick := range op.clock.Next(transport.One(arriving)) {
				elapsed = tick.Read()
			}

			shaped := elapsed

			if op.linear {
				shaped = 1 - elapsed
			}

			factor := shaped

			for out := range op.shape.Next(transport.Values(shaped)) {
				factor = out.Read()
			}

			if !yield(op.Carrier(value * factor)) {
				return
			}
		}
	}
}
