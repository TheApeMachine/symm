package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolLLHawkes     = nomagique.MustIntern("ll_hawkes")
	SymbolLLPoisson    = nomagique.MustIntern("ll_poisson")
	SymbolLLSelf       = nomagique.MustIntern("ll_self")
	SymbolDeltaPoisson = nomagique.MustIntern("ll_delta_poisson")
	SymbolDeltaSelf    = nomagique.MustIntern("ll_delta_self")
)

/*
Likelihood computes log-likelihood differentials against Poisson and self-only
baselines.
*/
func Likelihood(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	llHawkes, hasHawkes := input.Get(SymbolLLHawkes)
	llPoisson, hasPoisson := input.Get(SymbolLLPoisson)
	llSelf, hasSelf := input.Get(SymbolLLSelf)

	if !hasHawkes || !hasPoisson || !hasSelf || !finite(llHawkes, llPoisson, llSelf) {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: likelihood requires finite model values",
		)
	}

	deltaPoisson := llHawkes - llPoisson
	deltaSelf := llHawkes - llSelf

	if math.IsNaN(deltaPoisson) || math.IsInf(deltaPoisson, 0) {
		deltaPoisson = 0
	}

	if math.IsNaN(deltaSelf) || math.IsInf(deltaSelf, 0) {
		deltaSelf = 0
	}

	output := input
	output.Put(SymbolDeltaPoisson, deltaPoisson)
	output.Put(SymbolDeltaSelf, deltaSelf)

	return state, output, nil
}
