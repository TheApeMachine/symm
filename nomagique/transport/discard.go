package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Discard consumes a run without handing anything over.
*/
type Discard struct {
	*core.PrimitiveError
}

func NewDiscard() *Discard {
	return &Discard{PrimitiveError: core.NewPrimitiveError()}
}

func (discard *Discard) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(func(unsafe.Pointer) bool) {
		for range in {
		}
	}
}
