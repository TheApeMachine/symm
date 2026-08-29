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
func Lift(input *types.Frame) {
	value, hasValue := input.Get(types.SampleValue)
	baseline, hasBaseline := input.Get(SymbolBaseline)

	if !hasValue || !hasBaseline {
		input.Err = fmt.Errorf(
			"statistic: lift requires a value and a baseline",
		)

		return
	}

	input.Put(SymbolReady, 0)
	input.Put(SymbolResult, 0)

	if baseline > 0 {
		input.Put(SymbolResult, value/baseline-1)
		input.Put(SymbolReady, 1)
	}
}
