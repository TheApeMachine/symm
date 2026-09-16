package sequence

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

// One yields one borrowed address without copying its value.
type One struct {
	*core.PrimitiveError
	address unsafe.Pointer
}

func NewOne(address unsafe.Pointer) *One {
	return &One{PrimitiveError: core.NewPrimitiveError(), address: address}
}

func (one *One) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) { yield(one.address) }
}
