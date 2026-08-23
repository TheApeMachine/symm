package statistic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

var SymbolBaseline = types.MustIntern("baseline")

/*
Lift reports how far a value has risen above its baseline as a fraction of
the baseline itself: value over baseline minus one. The identity is the
honest zero — a value resting exactly on its baseline, or a first
observation whose baseline is itself, lifts nothing. A non-positive
baseline cannot ground a fraction, so the lift is not ready rather than
invented. The result slot is always emitted so Wire bindings remain
satisfied; consumers must gate on SymbolReady.
*/
func Lift(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	value, hasValue := input.Get(types.SampleValue)
	baseline, hasBaseline := input.Get(SymbolBaseline)

	if !hasValue || !hasBaseline {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: lift requires a value and a baseline",
		)
	}

	output := input
	output.Put(SymbolReady, 0)
	output.Put(SymbolResult, 0)

	if baseline > 0 {
		output.Put(SymbolResult, value/baseline-1)
		output.Put(SymbolReady, 1)
	}

	return state, output, nil
}
