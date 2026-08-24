package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Bonferroni emits the Bonferroni critical correlation threshold for a lag
search over SampleCount samples and SearchCount candidate lags:

	threshold = sqrt(2·ln(SearchCount + 1) / (SampleCount − 1))

It is a pure primitive over structural ports: PortA carries the search count,
PortB carries the sample count, and PortResult receives the threshold. The
tail factor 2 is the standard two-tailed Bonferroni correction for a lag
search, not an ad-hoc multiplier.
*/
func Bonferroni(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	searches, hasSearches := input.Get(types.PortA)
	samples, hasSamples := input.Get(types.PortB)

	if !hasSearches || !hasSamples {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: bonferroni requires a search count and a sample count",
		)
	}

	if !finite(searches, samples) {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: bonferroni requires finite operands",
		)
	}

	if searches < 0 {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: bonferroni search count cannot be negative",
		)
	}

	if samples <= 1 {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: bonferroni requires more than one sample",
		)
	}

	threshold := math.Sqrt(2 * math.Log(searches+1) / (samples - 1))
	output := input
	output.Put(types.PortResult, threshold)

	return state, output, nil
}
