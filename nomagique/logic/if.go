package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
If evaluates one predicate and exactly one selected branch. Any predicate or
branch error rolls the whole transition back to the caller's original state.
*/
func If(
	predicate types.Primitive,
	whenTrue types.Primitive,
	whenFalse types.Primitive,
) types.Primitive {
	return func(input *types.Frame) {
		// Any predicate or branch error rolls the whole transition back to
		// the caller's original state: predicate/branch run on a copy, and
		// input is overwritten with that copy only once the selected branch
		// has succeeded.
		predicateOutput := *input
		types.Step(predicate, &predicateOutput)

		if predicateOutput.Err != nil {
			*input = predicateOutput

			return
		}

		condition, found := predicateOutput.Get(SymbolCondition)

		if !found || !utils.IsFinite(condition) {
			predicateOutput.Err = fmt.Errorf("logic: if predicate must emit a finite condition")
			*input = predicateOutput

			return
		}

		branch := whenFalse

		if condition != 0 {
			branch = whenTrue
		}

		if branch == nil {
			*input = predicateOutput

			return
		}

		types.Step(branch, &predicateOutput)
		*input = predicateOutput
	}
}
