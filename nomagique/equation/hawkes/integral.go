package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IntegralInput reduces a response over each event's [lower,upper] ages.
*/
type IntegralInput struct {
	Parameters
	Side    float64
	Origin  float64
	Horizon float64
	Events  []Event
}

/*
Integral reduces a kernel-age response over events on one side before horizon.
*/
type Integral struct {
	core.Base[IntegralInput, float64]
	response func(beta, lower, upper float64) float64
}

func NewIntegral(response func(beta, lower, upper float64) float64) *Integral {
	return &Integral{response: response}
}

func (op *Integral) Next(
	in iter.Seq[core.Primitive[IntegralInput, IntegralInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			total := 0.0

			for _, event := range input.Events {
				if event.Side != input.Side || !(event.At < input.Horizon) {
					continue
				}

				lower := input.Origin - event.At

				if lower < 0 {
					lower = 0
				}

				upper := input.Horizon - event.At
				total += op.response(input.Beta, lower, upper)
			}

			if !yield(op.Carrier(total)) {
				return
			}
		}
	}
}
