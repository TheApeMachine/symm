package types

import (
	"fmt"
	"math"
	"math/bits"

	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Primitive is the universal reducer contract. Every operation receives immutable
state and input snapshots, then returns a candidate state, output, and error.
An error rejects the entire candidate transition.
*/
type Primitive func(
	state Frame,
	input Frame,
) (
	nextState Frame,
	output Frame,
	err error,
)

// Step executes one reducer transition and enforces rollback on error.
func Step(
	primitive Primitive,
	state Frame,
	input Frame,
) (Frame, Frame, error) {
	if primitive == nil {
		return state, Frame{}, PrimitiveError("primitive is nil")
	}

	nextState, output, err := primitive(state, input)

	if err != nil {
		return state, Frame{}, err
	}

	return nextState, output, nil
}

// Pipe chains primitives so each output becomes the following input.
func Pipe(primitives ...Primitive) Primitive {
	pipeline := append([]Primitive(nil), primitives...)

	return func(state Frame, input Frame) (Frame, Frame, error) {
		nextState := state
		output := input

		for _, primitive := range pipeline {
			var err error
			nextState, output, err = Step(primitive, nextState, output)

			if err != nil {
				return state, Frame{}, err
			}
		}

		return nextState, output, nil
	}
}

/*
Fork is true fan-out. Every branch observes the same original state and input.
Candidate state deltas are merged transactionally and conflicting writes fail;
outputs are overlaid in branch order for compatibility.
*/
func Fork(primitives ...Primitive) Primitive {
	branches := append([]Primitive(nil), primitives...)

	return func(state Frame, input Frame) (Frame, Frame, error) {
		nextState := state
		output := input

		for _, primitive := range branches {
			candidateState, branchOutput, err := Step(primitive, state, input)

			if err != nil {
				return state, Frame{}, err
			}

			nextState, err = mergeFrameChanges(state, nextState, candidateState, "fork state")

			if err != nil {
				return state, Frame{}, err
			}

			output.Merge(branchOutput)
		}

		return nextState, output, nil
	}
}

/*
ForkStrict is Fork with collision detection for both persistent state and output
facts. It is the preferred combinator for newly wired equations.
*/
func ForkStrict(primitives ...Primitive) Primitive {
	branches := append([]Primitive(nil), primitives...)

	return func(state Frame, input Frame) (Frame, Frame, error) {
		nextState := state
		output := input

		for _, primitive := range branches {
			candidateState, branchOutput, err := Step(primitive, state, input)

			if err != nil {
				return state, Frame{}, err
			}

			nextState, err = mergeFrameChanges(state, nextState, candidateState, "fork state")

			if err != nil {
				return state, Frame{}, err
			}

			output, err = mergeFrameChanges(input, output, branchOutput, "fork output")

			if err != nil {
				return state, Frame{}, err
			}
		}

		return nextState, output, nil
	}
}

/*
Configure runs a producer, copies one explicit control fact onto the original
input, and runs the consumer. Producer metrics survive; consumer output wins on
an intentional overlap.
*/
func Configure(producer Primitive, channel Symbol, consumer Primitive) Primitive {
	return func(state Frame, input Frame) (Frame, Frame, error) {
		nextState, controlOutput, err := Step(producer, state, input)

		if err != nil {
			return state, Frame{}, err
		}

		controlValue, found := controlOutput.Get(channel)

		if !found {
			return state, Frame{}, PrimitiveError("configure: control channel missing")
		}

		if !utils.IsFinite(controlValue) {
			return state, Frame{}, PrimitiveError("configure: control channel must be finite")
		}

		consumerInput := input
		consumerInput.Put(channel, controlValue)
		candidateState, consumerOutput, err := Step(consumer, nextState, consumerInput)

		if err != nil {
			return state, Frame{}, err
		}

		output := controlOutput
		output.Merge(consumerOutput)

		return candidateState, output, nil
	}
}

/*
Relay copies one named fact into another. It remains for source compatibility;
new compositions should use Wire so a primitive receives only explicit ports.

Deprecated: use Wire with explicit In and Out bindings.
*/
func Relay(from Symbol, to Symbol) Primitive {
	return func(state Frame, input Frame) (Frame, Frame, error) {
		value, found := input.Get(from)

		if !found {
			return state, Frame{}, PrimitiveError("relay: source slot missing")
		}

		output := input
		output.Put(to, value)

		return state, output, nil
	}
}

/*
Retained evaluates a projection over committed state and overlays that projection
onto the current input.
*/
func Retained(primitive Primitive) Primitive {
	return func(state Frame, input Frame) (Frame, Frame, error) {
		nextState, retained, err := Step(primitive, state, state)

		if err != nil {
			return state, Frame{}, err
		}

		output := input
		output.Merge(retained)

		return nextState, output, nil
	}
}

// Assign writes one explicit finite scalar without mutating state.
func Assign(symbol Symbol, value float64) Primitive {
	return func(state Frame, input Frame) (Frame, Frame, error) {
		if !utils.IsFinite(value) {
			return state, Frame{}, PrimitiveError("assign value must be finite")
		}

		output := input
		output.Put(symbol, value)

		return state, output, nil
	}
}

/*
Join evaluates source adapters against the same state and input, then merges
their facts. Persistent state collisions are rejected.
*/
func Join(primitives ...Primitive) Primitive {
	return Fork(primitives...)
}

// Identity passes input through unchanged.
func Identity(state Frame, input Frame) (Frame, Frame, error) {
	return state, input, nil
}

func mergeFrameChanges(
	base Frame,
	merged Frame,
	candidate Frame,
	context string,
) (Frame, error) {
	for maskIndex := range frameMaskWords {
		for remaining := base.Mask[maskIndex] | candidate.Mask[maskIndex]; remaining != 0; remaining &= remaining - 1 {
			bit := bits.TrailingZeros64(remaining)
			index := (maskIndex << 6) + bit
			symbol := Symbol(index)
			baseValue, baseFound := base.Get(symbol)
			candidateValue, candidateFound := candidate.Get(symbol)

			if sameFrameSlot(baseFound, baseValue, candidateFound, candidateValue) {
				continue
			}

			mergedValue, mergedFound := merged.Get(symbol)
			mergedChanged := !sameFrameSlot(
				baseFound,
				baseValue,
				mergedFound,
				mergedValue,
			)

			if mergedChanged && !sameFrameSlot(
				mergedFound,
				mergedValue,
				candidateFound,
				candidateValue,
			) {
				return base, fmt.Errorf(
					"nomagique: %s collision at %s",
					context,
					symbolLabel(symbol),
				)
			}

			if candidateFound {
				merged.Put(symbol, candidateValue)
			} else {
				merged.Delete(symbol)
			}
		}
	}

	return merged, nil
}

func sameFrameSlot(
	leftFound bool,
	left float64,
	rightFound bool,
	right float64,
) bool {
	return leftFound == rightFound &&
		(!leftFound || math.Float64bits(left) == math.Float64bits(right))
}
