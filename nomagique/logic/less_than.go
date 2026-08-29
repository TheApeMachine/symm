package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
LessThan emits one when A is strictly less than B.
*/
func LessThan(input *types.Frame) {
	left, hasLeft := input.Get(calculus.PortA)
	right, hasRight := input.Get(calculus.PortB)

	if !hasLeft || !hasRight || !utils.IsFinite(left) || !utils.IsFinite(right) {
		input.Err = fmt.Errorf("logic: less than requires finite left and right")

		return
	}

	condition := 0.0

	if left < right {
		condition = 1
	}

	input.Put(SymbolCondition, condition)
	input.Put(SymbolResult, condition)
}
