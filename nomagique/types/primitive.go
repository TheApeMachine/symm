package types

import (
	"fmt"
	"math"
	"math/bits"

	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Primitive is the universal reducer contract. A primitive receives one *Frame
(the current committed state and input, unified) and mutates it in place into
the next state and output, unified — never copying the 64KB+ Frame value on
call or return. Validation failures are recorded on frame.Err; a non-nil Err
short-circuits the enclosing pipeline.
*/
type Primitive func(frame *Frame)

/*
Step evaluates one primitive in place on frame. A primitive that fails sets
frame.Err; Step does not itself inspect or propagate it beyond running the
primitive, since frame is the caller's own storage.
*/
func Step(primitive Primitive, frame *Frame) {
	if primitive == nil {
		frame.Err = PrimitiveError("primitive is nil")

		return
	}

	primitive(frame)
}

/*
Pipe chains primitives so each mutates the same frame in sequence. A non-nil
Err stops the chain immediately, leaving frame as the failing primitive left
it.
*/
func Pipe(primitives ...Primitive) Primitive {
	pipeline := append([]Primitive(nil), primitives...)

	return func(frame *Frame) {
		for _, primitive := range pipeline {
			Step(primitive, frame)

			if frame.Err != nil {
				return
			}
		}
	}
}

/*
Fork fans one frame out to every branch and merges their results back into it.
Each branch receives its own copy of the pre-fork frame independently (a
branch must not see another branch's writes), and results overlay onto frame
in branch order.
*/
func Fork(primitives ...Primitive) Primitive {
	branches := append([]Primitive(nil), primitives...)

	return func(frame *Frame) {
		input := *frame

		for _, primitive := range branches {
			branch := input
			Step(primitive, &branch)

			if branch.Err != nil {
				*frame = branch

				return
			}

			frame.Merge(branch)
		}
	}
}

/*
TryFork is Fork for branches that may not yet have anything to compose: a
branch that fails without having changed the frame at all is dropped rather
than failing the whole step, so a composition of independently-arriving facts
tolerates the ones that have not arrived yet without losing the ones that
have. A branch that fails after writing or mutating even one slot is a genuine
defect, not an absence, and still propagates — checked with Equal, not a mask
comparison: a mature branch can fail after overwriting an already-populated
slot in place, which leaves the mask unchanged but the data mutated, and a
mask-only check would wrongly forgive that as untouched.
*/
func TryFork(primitives ...Primitive) Primitive {
	branches := append([]Primitive(nil), primitives...)

	return func(frame *Frame) {
		input := *frame

		for _, primitive := range branches {
			branch := input
			Step(primitive, &branch)

			if branch.Err != nil {
				if branch.Equal(input) {
					continue
				}

				*frame = branch

				return
			}

			frame.Merge(branch)
		}
	}
}

/*
ForkStrict is Fork with collision detection: conflicting writes to the same
slot fail rather than silently overwriting.
*/
func ForkStrict(primitives ...Primitive) Primitive {
	branches := append([]Primitive(nil), primitives...)

	return func(frame *Frame) {
		input := *frame
		output := input

		for _, primitive := range branches {
			branch := input
			Step(primitive, &branch)

			if branch.Err != nil {
				*frame = branch

				return
			}

			merged, err := mergeFrameChanges(input, output, branch, "fork")

			if err != nil {
				output.Err = err
				*frame = output

				return
			}

			output = merged
		}

		*frame = output
	}
}

/*
Configure runs a producer, copies one explicit control fact onto the frame,
and runs the consumer. Producer metrics survive; consumer output wins on an
intentional overlap.
*/
func Configure(producer Primitive, channel Symbol, consumer Primitive) Primitive {
	return func(frame *Frame) {
		Step(producer, frame)

		if frame.Err != nil {
			return
		}

		controlValue, found := frame.Get(channel)

		if !found {
			frame.Err = PrimitiveError("configure: control channel missing")

			return
		}

		if !utils.IsFinite(controlValue) {
			frame.Err = PrimitiveError("configure: control channel must be finite")

			return
		}

		Step(consumer, frame)
	}
}

/*
Relay copies one named fact into another. Deprecated in favor of Wire.
*/
func Relay(from Symbol, to Symbol) Primitive {
	return func(frame *Frame) {
		value, found := frame.Get(from)

		if !found {
			frame.Err = PrimitiveError("relay: source slot missing")

			return
		}

		frame.Put(to, value)
	}
}

/*
Assign writes one explicit finite scalar.
*/
func Assign(symbol Symbol, value float64) Primitive {
	return func(frame *Frame) {
		if !utils.IsFinite(value) {
			frame.Err = PrimitiveError("assign value must be finite")

			return
		}

		frame.Put(symbol, value)
	}
}

/*
Join merges branches like Fork; persistent collisions are rejected.
*/
func Join(primitives ...Primitive) Primitive {
	return Fork(primitives...)
}

/*
Identity leaves the frame unchanged.
*/
func Identity(frame *Frame) {}

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
