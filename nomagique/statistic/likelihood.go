package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolLLHawkes     = types.MustIntern("ll_hawkes")
	SymbolLLPoisson    = types.MustIntern("ll_poisson")
	SymbolLLSelf       = types.MustIntern("ll_self")
	SymbolDeltaPoisson = types.MustIntern("ll_delta_poisson")
	SymbolDeltaSelf    = types.MustIntern("ll_delta_self")
)

/*
Likelihood computes log-likelihood differentials against Poisson and self-only
baselines.
*/
func Likelihood(input types.Frame) types.Frame {
	llHawkes, hasHawkes := input.Get(SymbolLLHawkes)
	llPoisson, hasPoisson := input.Get(SymbolLLPoisson)
	llSelf, hasSelf := input.Get(SymbolLLSelf)

	if !hasHawkes || !hasPoisson || !hasSelf || !finite(llHawkes, llPoisson, llSelf) {
		input.Err = fmt.Errorf(
			"statistic: likelihood requires finite model values",
		)

		return input
	}

	deltaPoisson := llHawkes - llPoisson
	deltaSelf := llHawkes - llSelf

	if math.IsNaN(deltaPoisson) || math.IsInf(deltaPoisson, 0) {
		deltaPoisson = 0
	}

	if math.IsNaN(deltaSelf) || math.IsInf(deltaSelf, 0) {
		deltaSelf = 0
	}

	input.Put(SymbolDeltaPoisson, deltaPoisson)
	input.Put(SymbolDeltaSelf, deltaSelf)

	return input
}
