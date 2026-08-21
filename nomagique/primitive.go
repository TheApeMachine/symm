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
Configure wires a control channel between two primitives.

The producer runs on the incoming input and emits a control value in one named
slot. The consumer then runs on the ORIGINAL input with that slot overlaid, so
the producer contributes only the control parameter while the primary data
continues straight into the consumer unmodified.

	nomagique.Configure(temporal.Baseline, nmtypes.Span, temporal.Window)

Here Baseline reads the input and emits Span; Window still receives the original
input, now carrying Span, and uses it to size itself. Any slot-based control
parameter composes this way without signal-specific plumbing.
*/
func Configure(producer Primitive, channel Symbol, consumer Primitive) Primitive {
	return func(state Frame, input Frame) (Frame, Frame, error) {
		nextState, controlOutput, err := Step(producer, state, input)

		if err != nil {
			return state, Frame{}, err
		}

		controlValue, found := controlOutput.Get(channel)

		if !found {
			return state, Frame{}, primitiveError("configure: control channel missing")
		}

		consumerInput := input
		consumerInput.Put(channel, controlValue)

		nextState, output, err := Step(consumer, nextState, consumerInput)

		if err != nil {
			return state, Frame{}, err
		}

		// Merge the producer's output so every control metric it computed —
		// baseline value, stability, efficiency — survives alongside the
		// consumer's output instead of being discarded.
		output.Merge(controlOutput)

		return nextState, output, nil
	}
}

/*
Relay lifts one named slot into another, keeping every other slot intact. It is
the composition bridge between primitives that disagree on the slot naming of
the same number — for example routing a calculus result into the engine's
universal SampleValue slot so a downstream window or baseline can consume it.
*/
func Relay(from Symbol, to Symbol) Primitive {
	return func(state Frame, input Frame) (Frame, Frame, error) {
		value, found := input.Get(from)

		if !found {
			return state, Frame{}, primitiveError("relay: source slot missing")
		}

		output := input
		output.Put(to, value)

		return state, output, nil
	}
}

/*
Retained evaluates a primitive over committed state and overlays its projection
onto the current input. It is the causal bridge for estimators that must score
an observation against evidence that existed before that observation.
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

/*
Assign writes one configured scalar into the output without mutating state.
Constants such as algebraic identities and explicit control flags can thereby
participate in a visible composition rather than hiding in adapter functions.
*/
func Assign(symbol Symbol, value float64) Primitive {
	return func(state Frame, input Frame) (Frame, Frame, error) {
		output := input
		output.Put(symbol, value)

		return state, output, nil
	}
}

/*
Join evaluates two source adapters against the same input and merges their
output slots into one Frame before the downstream stage consumes it. It is the
dual-input combinator: two channels (for example the touch prices from one
source and the executed quantity from another) become a single numeric input
without either side owning the other's queue. State changes made by the first
adapter are visible to the second, and both outputs overlay deterministically.
*/
func Join(first Primitive, second Primitive) Primitive {
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
