package core

import (
	"errors"
	"sync/atomic"
)

var (
	ErrNotHeld    = errors.New("primitive held no value")
	ErrWrongType  = errors.New("primitive held the wrong type")
	ErrConversion = errors.New("primitive failed to convert")
	ErrDomain     = errors.New("primitive received a value outside its domain")
	ErrShape      = errors.New("primitive received an incompatible shape")
)

type PrimitiveError struct {
	err atomic.Pointer[error]
}

func NewPrimitiveError(errs ...error) *PrimitiveError {
	primitiveError := &PrimitiveError{}
	primitiveError.Error(errs...)
	return primitiveError
}

// Error retains all failures with atomic publication; reads do not allocate.
func (primitiveError *PrimitiveError) Error(errs ...error) error {
	for _, err := range errs {
		if err == nil {
			continue
		}

		for {
			previous := primitiveError.err.Load()
			var recorded error

			if previous != nil {
				recorded = *previous
			}

			joined := errors.Join(recorded, err)

			if primitiveError.err.CompareAndSwap(previous, &joined) {
				break
			}
		}
	}

	if recorded := primitiveError.err.Load(); recorded != nil {
		return *recorded
	}

	return nil
}
