package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolPValue            = types.MustIntern("correlation/p_value")
	SymbolSearchAdjustedP   = types.MustIntern("correlation/search_adjusted_p_value")
	SymbolFisherZ           = types.MustIntern("correlation/fisher_z")
	SymbolFisherStandardErr = types.MustIntern("correlation/fisher_standard_error")
	SymbolPValueReady       = types.MustIntern("correlation/pvalue_ready")
)

/*
PValue emits the Fisher-transform p-values of one correlation estimate. Under
the independent-return approximation, Z_F = atanh(r)·sqrt(N-3) is standard
normal under the null, p = 2(1-Φ(|Z_F|)), and the search-adjusted p-value is
the Bonferroni bound min(1, M·p) over M tested candidates.

Validity follows the honest-support rule: the p-values are emitted only when
the retained support is at least 4 (the Fisher standard error 1/sqrt(N-3) is
defined). Otherwise the primitive reports not ready; no significance cutoff is
embedded anywhere.
*/
func PValue(input types.Frame) types.Frame {
	correlationValue, hasCorrelation := input.Get(SymbolCorrelation)
	support, hasSupport := input.Get(SymbolSupport)

	if !hasCorrelation || !hasSupport || support < 4 || !finiteCorrelation(correlationValue) {
		input.Put(SymbolPValueReady, 0)

		return input
	}

	z := math.Atanh(correlationValue) * math.Sqrt(support-3)
	p := math.Erfc(math.Abs(z)/math.Sqrt2)

	input.Put(SymbolFisherZ, z)
	input.Put(SymbolFisherStandardErr, 1/math.Sqrt(support-3))
	input.Put(SymbolPValue, p)
	input.Put(SymbolPValueReady, 1)

	if searchCount, hasSearch := input.Get(SymbolSearchCount); hasSearch && searchCount >= 1 {
		input.Put(SymbolSearchAdjustedP, math.Min(1, searchCount*p))
	}

	return input
}

func finiteCorrelation(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
