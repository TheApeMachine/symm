package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
And emits one when both finite operands are non-zero.
*/
func And(input types.Frame) types.Frame {
	left, hasLeft := input.Get(calculus.PortA)
	right, hasRight := input.Get(calculus.PortB)

	if !hasLeft || !hasRight || !utils.IsFinite(left) || !utils.IsFinite(right) {
		input.Err = fmt.Errorf("logic: and requires finite left and right")

		return input
	}

	condition := 0.0

	if left != 0 && right != 0 {
		condition = 1
	}

	input.Put(SymbolCondition, condition)
	input.Put(SymbolResult, condition)

	return input
}
