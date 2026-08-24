package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Rule binds one predicate to the branch executed when it emits true.
*/
type Rule struct {
	When types.Primitive
	Then types.Primitive
}

/*
Circuit evaluates ordered rules and executes the first matching branch. Every
failure rejects all candidate predicate state accumulated during this call.
*/
func Circuit(rules []Rule, fallback types.Primitive) types.Primitive {
	program := append([]Rule(nil), rules...)

	return func(input types.Frame) types.Frame {
		for _, rule := range program {
			if rule.When == nil {
				input.Err = fmt.Errorf("logic: circuit rule requires a predicate")

				return input
			}

			candidate := types.Step(rule.When, input)

			if candidate.Err != nil {
				return candidate
			}

			condition, found := candidate.Get(SymbolCondition)

			if !found || !utils.IsFinite(condition) {
				candidate.Err = fmt.Errorf("logic: circuit predicate must emit a finite condition")

				return candidate
			}

			input = candidate

			if condition == 0 {
				continue
			}

			if rule.Then == nil {
				return input
			}

			branch := types.Step(rule.Then, input)

			if branch.Err != nil {
				return branch
			}

			return branch
		}

		if fallback == nil {
			return input
		}

		return types.Step(fallback, input)
	}
}
