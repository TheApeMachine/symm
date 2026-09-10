package core

import "errors"

var (
	ErrNotHeld      = errors.New("primitive held no value")
	ErrWrongType    = errors.New("primitive held the wrong type")
	ErrConversion   = errors.New("primitive failed to convert")
	ErrDomain       = errors.New("primitive received a value outside its domain")
	ErrShape        = errors.New("primitive received an incompatible shape")
	ErrDivideByZero = errors.New("primitive received a zero value for division")
)

// PrimitiveError is the one failure accumulator. Reading errors has no side
// effects; recording nil does not grow the error tree. Joined branch failures
// are admitted individually so overlapping propagation cannot duplicate their
// entire subtrees. Single-error wrappers retain their descriptive context.
type PrimitiveError struct{ err error }

func (state *PrimitiveError) Error(errs ...error) error {
	for _, err := range errs {
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			state.Error(joined.Unwrap()...)
			continue
		}

		if err != nil && !errors.Is(state.err, err) {
			state.err = errors.Join(state.err, err)
		}
	}
	return state.err
}
