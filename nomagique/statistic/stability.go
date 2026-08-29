package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolStability = types.MustIntern("stability")
	SymbolRange     = types.MustIntern("range")
)

/*
Stability returns the primitive that measures dimensionless clustering around
a supplied mean for one series' retained ring. The prefix namespaces every
slot; the empty prefix keeps the legacy generic slots. It emits one for a
collapsed range and otherwise one minus the largest departure from the mean
divided by the collection's own range.
*/
func Stability(prefix string) types.Primitive {
	series := temporal.NewSeries(prefix)
	stabilitySymbol := types.MustIntern(temporal.JoinPrefix(prefix, "stability"))
	rangeSymbol := types.MustIntern(temporal.JoinPrefix(prefix, "range"))
	meanSymbol := types.MustIntern(temporal.JoinPrefix(prefix, "mean"))

	return func(input *types.Frame) {
		count := series.Count(*input)

		if count < minimumStabilitySamples {
			input.Put(stabilitySymbol, 0)
			input.Put(series.ReadySymbol, 0)

			return
		}

		center, found := input.Get(meanSymbol)

		if !found {
			input.Err = fmt.Errorf(
				"statistic: stability requires a mean",
			)

			return
		}

		minimum := math.MaxFloat64
		maximum := -math.MaxFloat64
		maximumDeparture := 0.0

		for index := 0; index < count; index++ {
			value, populated := series.SampleAt(input, index)

			if !populated {
				continue
			}

			minimum = math.Min(minimum, value)
			maximum = math.Max(maximum, value)
			maximumDeparture = math.Max(maximumDeparture, math.Abs(value-center))
		}

		rangeValue := maximum - minimum
		stability := 1.0

		if rangeValue > 0 {
			stability = 1 - maximumDeparture/rangeValue
			stability = math.Max(0, math.Min(1, stability))
		}

		input.Put(rangeSymbol, rangeValue)
		input.Put(stabilitySymbol, stability)
		input.Put(series.ReadySymbol, 1)
	}
}

const minimumStabilitySamples = 2
