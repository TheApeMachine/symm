package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Decay walks a retained level toward zero. Input level replaces state when
present. Clock supplies linear elapsed progress; shape, when present, is the
explicit remaining multiplier.
*/
func Decay(input *types.Frame) {
	level, hasLevel := input.Get(SymbolLevel)

	if !hasLevel || !utils.IsFinite(level) {
		input.Err = fmt.Errorf("calculus: decay requires a finite level")

		return
	}

	remaining := 0.0

	if clock, hasClock := input.Get(SymbolClock); hasClock {
		if !utils.IsFinite(clock) {
			input.Err = fmt.Errorf("calculus: decay clock must be finite")

			return
		}

		remaining = 1 - clock

		if remaining < 0 {
			remaining = 0
		}
	}

	if shape, hasShape := input.Get(SymbolShape); hasShape {
		if !utils.IsFinite(shape) {
			input.Err = fmt.Errorf("calculus: decay shape must be finite")

			return
		}

		remaining = shape
	}

	result := level * remaining

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: decay overflowed")

		return
	}

	input.Put(SymbolLevel, result)
	input.Put(PortResult, result)
}
