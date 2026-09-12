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

type PrimitiveError struct {
	err error
}

func NewPrimitiveError() *PrimitiveError {
	return &PrimitiveError{}
}

func (pe *PrimitiveError) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			pe.err = errors.Join(pe.err, err)
		}
	}

	return pe.err
}
