package core

import (
	"errors"

	"github.com/theapemachine/errnie"
)

/*
The failures a Primitive can carry. They are typed so a consumer can tell a
composition that was wired wrong from one that met bad data, and act on the
difference rather than on a string.
*/
var (
	// ErrNotHeld means a Primitive held nothing where a value was required.
	ErrNotHeld = errors.New("primitive held no value")

	// ErrWrongType means a Primitive held something of an unexpected type,
	// which is a composition error: two Primitives were joined that do not
	// speak about the same thing.
	ErrWrongType = errors.New("primitive held the wrong type")

	ErrConversion = errors.New("primitive failed to convert")
)

/*
PrimitiveError gives every Primitive a place to carry a failure without
threading errors through returns. Embedding it satisfies the contract's Error
method, so a Primitive declares nothing to get one, and a consumer checks a
whole pipeline once at the end rather than after every step.

The receiver is a pointer because recording a failure writes to the Primitive
itself. A value receiver would collect errors into a copy and drop them.
*/
type PrimitiveError struct {
	err error
}

/*
Error both asks and tells. Called with nothing it reports what this Primitive
has failed at, and a nil answer means it is sound. Called with errors it
records them, keeping whatever it already carried, because a composition can
break in several places for unrelated reasons and collapsing those to one
throws away the part a reader needs.

Joining a nil changes nothing, so a caller passes on whatever it has without
checking it first.
*/
func (primitive *PrimitiveError) Error(errs ...error) error {
	primitive.err = errors.Join(append(errs, primitive.err)...)
	return errnie.Error(primitive.err)
}
