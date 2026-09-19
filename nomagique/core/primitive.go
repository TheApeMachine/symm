package core

import (
	"iter"
	"unsafe"
)

// Primitive is the fundamental interface for streaming components.
type Primitive interface {
	Next(iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer]
	Error(...error) error
}

type PrimitiveError struct {
	err error
}

func NewPrimitiveError() *PrimitiveError {
	return &PrimitiveError{}
}

func (p *PrimitiveError) Error(errs ...error) error {
	if len(errs) > 0 {
		p.err = errs[0]
	}
	return p.err
}
