package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Attack applies a finite jump to a finite base level.
*/
func Attack(input types.Frame) types.Frame {
	base, hasBase := input.Get(SymbolBase)
	jump, hasJump := input.Get(SymbolJump)

	if !hasBase || !hasJump || !utils.IsFinite(base) || !utils.IsFinite(jump) {
		input.Err = fmt.Errorf("calculus: attack requires finite base and jump")

		return input
	}

	result := base + jump

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: attack overflowed")

		return input
	}

	input.Put(SymbolBase, result)
	input.Put(PortResult, result)

	return input
}
