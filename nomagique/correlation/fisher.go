package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
minimumFisherSupport is the smallest support at which the Fisher standard
error 1/sqrt(N-3) is defined. Below it, significance cannot be stated.
*/
const minimumFisherSupport = 4

/*
Fisher reports the significance of a correlation through the Fisher
z-transform.

Under the independent-return approximation Z = atanh(r)·sqrt(N-3) is standard
normal under the null, so p = erfc(|Z|/sqrt(2)) is the two-sided tail mass.
Where the correlation came from a search over M candidates, the Bonferroni
bound min(1, M·p) corrects for having taken the best of many looks.

The carrier in is the correlation; the carrier out is the p-value. Support
and SearchCount are slots, so the same node serves a single estimate or a
searched one. No significance cutoff is embedded: Fisher reports, the
consumer decides.

Degenerate behavior: an omitted Support cannot establish significance and
yields 0; an omitted SearchCount means no search correction.
*/
type Fisher struct {
	Support     types.Node
	SearchCount types.Node

	z             types.Number
	standardError types.Number
	pValue        types.Number
	adjusted      types.Number
	hasAdjusted   bool
	ready         bool
}

func (fisher *Fisher) Step(correlation types.Number) types.Number {
	fisher.reset()

	if fisher.Support == nil {
		return 0
	}

	support := float64(fisher.Support.Step(correlation))
	value := float64(correlation)

	if support < minimumFisherSupport ||
		math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) >= 1 {
		return 0
	}

	degreesOfFreedom := math.Sqrt(support - 3)

	fisher.z = types.Number(math.Atanh(value) * degreesOfFreedom)
	fisher.standardError = types.Number(1 / degreesOfFreedom)
	fisher.pValue = types.Number(math.Erfc(math.Abs(float64(fisher.z)) / math.Sqrt2))
	fisher.ready = true

	if fisher.SearchCount != nil {
		candidates := fisher.SearchCount.Step(correlation)

		if candidates >= 1 {
			fisher.adjusted = types.Number(math.Min(1, float64(candidates*fisher.pValue)))
			fisher.hasAdjusted = true
		}
	}

	return fisher.pValue
}

func (fisher *Fisher) reset() {
	fisher.z = 0
	fisher.standardError = 0
	fisher.pValue = 0
	fisher.adjusted = 0
	fisher.hasAdjusted = false
	fisher.ready = false
}

// Ready reports whether the last step produced defined statistics.
func (fisher *Fisher) Ready() bool { return fisher.ready }

// Z returns the Fisher-transformed test statistic.
func (fisher *Fisher) Z() types.Number { return fisher.z }

// StandardError returns the Fisher standard error 1/sqrt(N-3).
func (fisher *Fisher) StandardError() types.Number { return fisher.standardError }

// PValue returns the two-sided tail mass under the null.
func (fisher *Fisher) PValue() types.Number { return fisher.pValue }

/*
SearchAdjustedPValue returns the Bonferroni-corrected p-value, reporting
false when the estimate did not come from a search.
*/
func (fisher *Fisher) SearchAdjustedPValue() (types.Number, bool) {
	return fisher.adjusted, fisher.hasAdjusted
}

var _ types.Node = (*Fisher)(nil)
