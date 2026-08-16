package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolResult = nomagique.MustIntern("result")
	SymbolReady  = nomagique.MustIntern("ready")
	SymbolCount  = nomagique.MustIntern("count")
)

func collectSamples(
	input *nomagique.Frame,
	primitive string,
) ([nomagique.MaxSamples]float64, int, error) {
	values := [nomagique.MaxSamples]float64{}
	count := 0

	for index := range nomagique.MaxSamples {
		symbol := nomagique.MustSampleSymbol(index)
		value, found := input.Get(symbol)

		if !found {
			continue
		}

		if math.IsNaN(value) || math.IsInf(value, 0) {
			return values, 0, fmt.Errorf(
				"statistic: %s sample/%d must be finite",
				primitive,
				index,
			)
		}

		values[count] = value
		count++
	}

	return values, count, nil
}

func sortSamples(values *[nomagique.MaxSamples]float64, count int) {
	for index := 1; index < count; index++ {
		value := values[index]
		position := index

		for position > 0 && values[position-1] > value {
			values[position] = values[position-1]
			position--
		}

		values[position] = value
	}
}
