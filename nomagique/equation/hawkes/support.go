package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Event is one marked arrival. Side is 0 or 1.
*/
type Event struct {
	Side float64
	At   float64
}

/*
SupportInput is the history used to reduce a kernel before a horizon.
*/
type SupportInput struct {
	Side    float64
	Beta    float64
	Horizon float64
	Events  []Event
}

/*
Support reduces exp(-beta*age) over events strictly before horizon on the
selected side. Equal-time events do not excite one another.
*/
type Support struct {
	core.Base[SupportInput, float64]
}

func NewSupport() *Support {
	return &Support{}
}

func (op *Support) Next(
	in iter.Seq[core.Primitive[SupportInput, SupportInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			total := 0.0

			for _, event := range input.Events {
				if event.Side != input.Side || !(event.At < input.Horizon) {
					continue
				}

				total += kernel(input.Beta, input.Horizon-event.At)
			}

			if !yield(op.Carrier(total)) {
				return
			}
		}
	}
}
