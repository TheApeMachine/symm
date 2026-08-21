package logic

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
)

/*
Mux selects one of the two living operand slots from an explicit condition.
It writes the selected value to SymbolResult and preserves every input slot.
*/
func Mux(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	condition, hasCondition := input.Get(SymbolCondition)
	left, hasLeft := input.Get(calculus.SymbolLeft)
	right, hasRight := input.Get(calculus.SymbolRight)

	if !hasCondition || !hasLeft || !hasRight {
		return state, nomagique.Frame{}, conditionError("mux operands")
	}

	result := right

	if condition != 0 {
		result = left
	}

	output := input
	output.Put(SymbolResult, result)

	return state, output, nil
}

/*
If evaluates an explicit predicate and routes the resulting Frame through one
of two branches. The predicate must emit SymbolCondition; readiness is never
guessed from unrelated slots.
*/
func If(
	predicate nomagique.Primitive,
	whenTrue nomagique.Primitive,
	whenFalse nomagique.Primitive,
) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		nextState, output, err := nomagique.Step(predicate, state, input)

		if err != nil {
			return state, nomagique.Frame{}, err
		}

		condition, found := output.Get(SymbolCondition)

		if !found {
			return state, nomagique.Frame{}, conditionError("if predicate")
		}

		branch := whenFalse

		if condition != 0 {
			branch = whenTrue
		}

		if branch == nil {
			return nextState, output, nil
		}

		return nomagique.Step(branch, nextState, output)
	}
}

/*
Rule binds one predicate to the branch executed when it emits true.
*/
type Rule struct {
	When nomagique.Primitive
	Then nomagique.Primitive
}

/*
Circuit evaluates ordered rules and executes the first matching branch.
Predicates commit their state in order so stateful empirical gates continue
learning even when their branch does not fire.
*/
func Circuit(rules []Rule, fallback nomagique.Primitive) nomagique.Primitive {
	program := append([]Rule(nil), rules...)

	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		nextState := state
		output := input

		for _, rule := range program {
			if rule.When == nil {
				continue
			}

			candidateState, candidateOutput, err := nomagique.Step(
				rule.When,
				nextState,
				output,
			)

			if err != nil {
				return state, nomagique.Frame{}, err
			}

			nextState = candidateState
			output = candidateOutput
			condition, found := output.Get(SymbolCondition)

			if !found {
				return state, nomagique.Frame{}, conditionError("circuit predicate")
			}

			if condition == 0 {
				continue
			}

			if rule.Then == nil {
				return nextState, output, nil
			}

			return nomagique.Step(rule.Then, nextState, output)
		}

		if fallback == nil {
			return nextState, output, nil
		}

		return nomagique.Step(fallback, nextState, output)
	}
}
