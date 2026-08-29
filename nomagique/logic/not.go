package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Not inverts one finite numeric condition.
*/
func Not(input *types.Frame) {
	condition, found := input.Get(SymbolCondition)

	if !found || !utils.IsFinite(condition) {
		input.Err = fmt.Errorf("logic: not requires a finite condition")

		return
	}

	result := 0.0

	if condition == 0 {
		result = 1
	}

	input.Put(SymbolCondition, result)
	input.Put(SymbolResult, result)
}
