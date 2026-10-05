package store

import (
	"iter"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Radix associates values with ordered keys. It never mutates its configured
source: each arrival is applied to a tree and the next tree is handed over.
*/
type Radix struct {
	*core.PrimitiveError
	root *iradix.Tree[[]byte]
}

func NewRadix() *Radix {
	return &Radix{
		PrimitiveError: core.NewPrimitiveError(),
		root:           iradix.New[[]byte](),
	}
}

func (op *Radix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
		}
	}
}
