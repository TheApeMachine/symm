package probability

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolResult = nomagique.MustIntern("result")
	SymbolCount  = nomagique.MustIntern("count")
)

/*
Geomean combines positive finite values stored in generic sample slots.
*/
func Geomean(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	count := 0
	logSum := 0.0

	for index := range nomagique.MaxSamples {
		symbol := nomagique.MustSampleSymbol(index)
		value, found := input.Get(symbol)

		if !found {
			continue
		}

		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return state, nomagique.Frame{}, fmt.Errorf(
				"probability: geomean sample/%d must be positive and finite",
				index,
			)
		}

		logSum += math.Log(value)
		count++
	}

	if count == 0 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"probability: geomean requires at least one sample",
		)
	}

	output := input
	output.Put(SymbolResult, math.Exp(logSum/float64(count)))
	output.Put(SymbolCount, float64(count))

	return state, output, nil
}
