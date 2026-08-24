package types

import (
	"fmt"
	"math"
	"math/bits"

	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Primitive is the universal reducer contract. A primitive takes one Frame (the
current committed state and input, unified) and returns one Frame (the next
state and output, unified). Validation failures are recorded on the returned
Frame's Err field; a non-nil Err short-circuits the enclosing pipeline.
*/
type Primitive func(input Frame) Frame

/*
Step evaluates one primitive. A primitive that fails returns the input Frame
with Err set, so Step simply propagates it.
*/
func Step(primitive Primitive, input Frame) Frame {
	if primitive == nil {
		input.Err = PrimitiveError("primitive is nil")

		return input
	}

	return primitive(input)
}

/*
Pipe chains primitives so each frame flows into the next. A non-nil Err stops
the chain immediately and propagates the errored frame unchanged.
*/
func Pipe(primitives ...Primitive) Primitive {
	pipeline := append([]Primitive(nil), primitives...)

	return func(input Frame) Frame {
		output := input

		for _, primitive := range pipeline {
			output = Step(primitive, output)

			if output.Err != nil {
				return output
			}
		}

		return output
	}
}

/*
Fork fans one frame out to every branch and merges their frames. Each branch
receives the same input independently; results overlay in branch order.
*/
func Fork(primitives ...Primitive) Primitive {
	branches := append([]Primitive(nil), primitives...)

	return func(input Frame) Frame {
		output := input

		for _, primitive := range branches {
			branch := Step(primitive, input)

			if branch.Err != nil {
				return branch
			}

			output.Merge(branch)
		}

		return output
	}
}

/*
ForkStrict is Fork with collision detection: conflicting writes to the same
slot fail rather than silently overwriting.
*/
func ForkStrict(primitives ...Primitive) Primitive {
	branches := append([]Primitive(nil), primitives...)

	return func(input Frame) Frame {
		output := input

		for _, primitive := range branches {
			branch := Step(primitive, input)

			if branch.Err != nil {
				return branch
			}

			merged, err := mergeFrameChanges(input, output, branch, "fork")

			if err != nil {
				output.Err = err

				return output
			}

			output = merged
		}

		return output
	}
}

/*
Configure runs a producer, copies one explicit control fact onto the input, and
runs the consumer. Producer metrics survive; consumer output wins on an
intentional overlap.
*/
func Configure(producer Primitive, channel Symbol, consumer Primitive) Primitive {
	return func(input Frame) Frame {
		control := Step(producer, input)

		if control.Err != nil {
			return control
		}

		controlValue, found := control.Get(channel)

		if !found {
			input.Err = PrimitiveError("configure: control channel missing")

			return input
		}

		if !utils.IsFinite(controlValue) {
			input.Err = PrimitiveError("configure: control channel must be finite")

			return input
		}

		consumerInput := control

		return Step(consumer, consumerInput)
	}
}

/*
Relay copies one named fact into another. Deprecated in favor of Wire.
*/
func Relay(from Symbol, to Symbol) Primitive {
	return func(input Frame) Frame {
		value, found := input.Get(from)

		if !found {
			input.Err = PrimitiveError("relay: source slot missing")

			return input
		}

		input.Put(to, value)

		return input
	}
}

/*
Assign writes one explicit finite scalar.
*/
func Assign(symbol Symbol, value float64) Primitive {
	return func(input Frame) Frame {
		if !utils.IsFinite(value) {
			input.Err = PrimitiveError("assign value must be finite")

			return input
		}

		input.Put(symbol, value)

		return input
	}
}

/*
Join merges branches like Fork; persistent collisions are rejected.
*/
func Join(primitives ...Primitive) Primitive {
	return Fork(primitives...)
}

/*
Identity passes the input frame through unchanged.
*/
func Identity(input Frame) Frame {
	return input
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
