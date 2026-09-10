package adaptive

import (
	"iter"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
pathRetention owns the configured mean-shift policy for accepted observations.
*/
type pathRetention struct {
	core.Base[[]equation.Price, []equation.Price]
	window *Window
}

/*
NewPath composes timestamp acceptance with adaptive observation retention. The
configured window determines retained support from changes in the observed
values. No wall-clock expiry or fixed history length is imposed. Regressed
observations never reach the policy; restatements are accepted observations.
*/
func NewPath(window *Window) *correlation.Path {
	return correlation.NewPath(&pathRetention{window: window})
}

func (op *pathRetention) Next(
	in iter.Seq[core.Primitive[[]equation.Price, []equation.Price]],
) iter.Seq[core.Primitive[[]equation.Price, []equation.Price]] {
	return func(yield func(core.Primitive[[]equation.Price, []equation.Price]) bool) {
		for arriving := range in {
			observations := arriving.Read()

			if len(observations) == 0 {
				op.Error(core.ErrShape)
				return
			}

			reading := op.window.Observe(observations[len(observations)-1].Value)
			start := max(0, len(observations)-int(reading.Capacity))

			if start == 0 {
				if !yield(op.Carrier(observations)) {
					return
				}

				continue
			}

			if !yield(op.Carrier(slices.Clone(observations[start:]))) {
				return
			}
		}
	}
}
