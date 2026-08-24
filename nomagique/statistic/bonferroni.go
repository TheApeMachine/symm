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
func Bonferroni(input types.Frame) types.Frame {
	searches, hasSearches := input.Get(types.PortA)
	samples, hasSamples := input.Get(types.PortB)

	if !hasSearches || !hasSamples {
		input.Err = fmt.Errorf(
			"statistic: bonferroni requires a search count and a sample count",
		)

		return input
	}

	if !finite(searches, samples) {
		input.Err = fmt.Errorf(
			"statistic: bonferroni requires finite operands",
		)

		return input
	}

	if searches < 0 {
		input.Err = fmt.Errorf(
			"statistic: bonferroni search count cannot be negative",
		)

		return input
	}

	if samples <= 1 {
		input.Err = fmt.Errorf(
			"statistic: bonferroni requires more than one sample",
		)

		return input
	}

	threshold := math.Sqrt(2 * math.Log(searches+1) / (samples - 1))
	input.Put(types.PortResult, threshold)

	return input
}
