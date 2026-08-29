package calculus

import (
	"fmt"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Attack applies a finite jump to a finite base level.
*/
func Attack(input *types.Frame) {
	base, hasBase := input.Get(SymbolBase)
	jump, hasJump := input.Get(SymbolJump)

	if !hasBase || !hasJump || !utils.IsFinite(base) || !utils.IsFinite(jump) {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: attack requires finite base and jump",
			nil,
		))

		return
	}

	result := base + jump

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: attack overflowed")

		return
	}

	input.Put(SymbolBase, result)
	input.Put(PortResult, result)
}
