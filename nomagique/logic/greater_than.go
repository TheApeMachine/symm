package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
GreaterThan emits one when A is strictly greater than B.
*/
func GreaterThan(input types.Frame) types.Frame {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("logic: greater than requires finite a and b")

		return input
	}

	condition := 0.0

	if a > b {
		condition = 1
	}

	input.Put(SymbolCondition, condition)
	input.Put(SymbolResult, condition)

	return input
}
