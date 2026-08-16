package nomagique

/*
Primitive is the universal reducer contract. Every operation receives an
immutable state snapshot and an input Frame, then returns the candidate state,
output, and any validation failure.
*/
type Primitive func(
	state Frame,
	input Frame,
) (
	nextState Frame,
	output Frame,
	err error,
)

/*
Step executes one reducer transition.
*/
func Step(
	primitive Primitive,
	state Frame,
	input Frame,
) (Frame, Frame, error) {
	if primitive == nil {
		return state, Frame{}, primitiveError("primitive is nil")
	}

	return primitive(state, input)
}

/*
Pipe chains primitives so each output becomes the following primitive's input.
All reducers share one universal state Frame and should preserve unrelated slots.
*/
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
Fork evaluates two reducers against the same input. The second reducer observes
state changes made by the first, and its output deterministically overlays the
first output. This keeps fan-out allocation-free and avoids goroutine scheduling
inside numeric hot paths.
*/
func Fork(first Primitive, second Primitive) Primitive {
	return func(state Frame, input Frame) (Frame, Frame, error) {
		nextState, firstOutput, err := Step(first, state, input)

		if err != nil {
			return state, Frame{}, err
		}

		nextState, secondOutput, err := Step(second, nextState, input)

		if err != nil {
			return state, Frame{}, err
		}

		firstOutput.Merge(secondOutput)

		return nextState, firstOutput, nil
	}
}

/*
Identity passes input through unchanged.
*/
func Identity(state Frame, input Frame) (Frame, Frame, error) {
	return state, input, nil
}
