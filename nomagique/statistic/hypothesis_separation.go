package statistic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
HypothesisSeparation emits the normalized signal-to-noise margin between the
strongest and second-strongest of the named competing hypothesis slots:

	separation = (dominant − runnerUp) / dominant

No evidence separates nothing; one supported hypothesis separates completely;
equal support has zero separation. The result lands in SymbolResult, which is
the same interned slot as the structural PortResult.

The competing hypothesis symbols are bound at composition time by the caller,
so this primitive carries no knowledge of what those hypotheses mean.
*/
func HypothesisSeparation(symbols ...types.Symbol) types.Primitive {
	hypotheses := append([]types.Symbol(nil), symbols...)

	return func(
		state types.Frame,
		input types.Frame,
	) (types.Frame, types.Frame, error) {
		dominant := 0.0
		runnerUp := 0.0
		count := 0

		for _, symbol := range hypotheses {
			value, found := input.Get(symbol)

			if !found {
				continue
			}

			if !finite(value) {
				return state, types.Frame{}, fmt.Errorf(
					"statistic: hypothesis separation requires finite scores",
				)
			}

			count++

			if value >= dominant {
				runnerUp = dominant
				dominant = value
				continue
			}

			if value > runnerUp {
				runnerUp = value
			}
		}

		separation := 0.0

		if dominant > 0 && count >= 2 {
			separation = (dominant - runnerUp) / dominant
		}

		output := input
		output.Put(SymbolResult, separation)

		return state, output, nil
	}
}
