package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
GreaterOrEqual emits one when A is greater than or equal to B.
*/
func GreaterOrEqual(input types.Frame) types.Frame {
	left, hasA := input.Get(calculus.PortA)
	right, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(left) || !utils.IsFinite(right) {
		input.Err = fmt.Errorf("logic: greater or equal requires finite left and right")

		return input
	}

	condition := 0.0

	if left >= right {
		condition = 1
	}

	input.Put(SymbolCondition, condition)
	input.Put(SymbolResult, condition)

	return input
}
