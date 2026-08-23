package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

// Mux selects A when condition is non-zero, otherwise B.
func Mux(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	condition, hasCondition := input.Get(SymbolCondition)
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasCondition || !hasA || !hasB || !utils.IsFinite(condition) ||
		!utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("logic: mux requires finite condition, a, and b")
	}

	result := b

	if condition != 0 {
		result = a
	}

	output := input
	output.Put(SymbolResult, result)

	return state, output, nil
}

/*
If evaluates one predicate and exactly one selected branch. Any predicate or
branch error rolls the whole transition back to the caller's original state.
*/
func If(
	predicate nomagique.Primitive,
	whenTrue nomagique.Primitive,
	whenFalse nomagique.Primitive,
) nomagique.Primitive {
	return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		predicateState, predicateOutput, err := nomagique.Step(predicate, state, input)

		if err != nil {
			return state, types.Frame{}, err
		}

		condition, found := predicateOutput.Get(SymbolCondition)

		if !found || !utils.IsFinite(condition) {
			return state, types.Frame{}, fmt.Errorf("logic: if predicate must emit a finite condition")
		}

		branch := whenFalse

		if condition != 0 {
			branch = whenTrue
		}

		if branch == nil {
			return predicateState, predicateOutput, nil
		}

		nextState, output, err := nomagique.Step(branch, predicateState, predicateOutput)

		if err != nil {
			return state, types.Frame{}, err
		}

		return nextState, output, nil
	}
}

// Rule binds one predicate to the branch executed when it emits true.
type Rule struct {
	When nomagique.Primitive
	Then nomagique.Primitive
}

/*
Circuit evaluates ordered rules and executes the first matching branch. Every
failure rejects all candidate predicate state accumulated during this call.
*/
func Circuit(rules []Rule, fallback nomagique.Primitive) nomagique.Primitive {
	program := append([]Rule(nil), rules...)

	return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		nextState := state
		output := input

		for _, rule := range program {
			if rule.When == nil {
				continue
			}

			candidateState, candidateOutput, err := nomagique.Step(rule.When, nextState, output)

			if err != nil {
				return state, types.Frame{}, err
			}

			condition, found := candidateOutput.Get(SymbolCondition)

			if !found || !utils.IsFinite(condition) {
				return state, types.Frame{}, fmt.Errorf("logic: circuit predicate must emit a finite condition")
			}

			nextState = candidateState
			output = candidateOutput

			if condition == 0 {
				continue
			}

			if rule.Then == nil {
				return nextState, output, nil
			}

			branchState, branchOutput, err := nomagique.Step(rule.Then, nextState, output)

			if err != nil {
				return state, types.Frame{}, err
			}

			return branchState, branchOutput, nil
		}

		if fallback == nil {
			return nextState, output, nil
		}

		fallbackState, fallbackOutput, err := nomagique.Step(fallback, nextState, output)

		if err != nil {
			return state, types.Frame{}, err
		}

		return fallbackState, fallbackOutput, nil
	}
}
