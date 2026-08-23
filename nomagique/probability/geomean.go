package probability

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolResult = types.MustIntern("result")
	SymbolCount  = types.MustIntern("count")
)

/*
Geomean combines positive finite values stored in generic sample slots.
*/
func Geomean(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	count := 0
	logSum := 0.0

	for index := range types.MaxSamples {
		symbol := types.MustSampleSymbol(index)
		value, found := input.Get(symbol)

		if !found {
			continue
		}

		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return state, types.Frame{}, fmt.Errorf(
				"probability: geomean sample/%d must be positive and finite",
				index,
			)
		}

		logSum += math.Log(value)
		count++
	}

	if count == 0 {
		return state, types.Frame{}, fmt.Errorf(
			"probability: geomean requires at least one sample",
		)
	}

	output := input
	output.Put(SymbolResult, math.Exp(logSum/float64(count)))
	output.Put(SymbolCount, float64(count))

	return state, output, nil
}
