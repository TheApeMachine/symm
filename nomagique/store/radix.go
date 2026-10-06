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

Arrivals are map[string][]byte. "selector" names the key; when "data" is
present it is inserted under that key. Every arrival yields the current tree,
so a selector without data reads the tree a prefix walk starts from.
*/
type Radix struct {
	*core.PrimitiveError
	root *iradix.Tree[[]byte]
}

func NewRadix(root ...*iradix.Tree[[]byte]) *Radix {
	tree := iradix.New[[]byte]()

	if len(root) > 0 && root[0] != nil {
		tree = root[0]
	}

	return &Radix{
		PrimitiveError: core.NewPrimitiveError(),
		root:           tree,
	}
}

func (op *Radix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			command := *(*map[string][]byte)(arriving)
			selector, held := command["selector"]

			if !held {
				op.Error(core.ErrNotHeld)
				return
			}

			if value, write := command["data"]; write {
				op.root, _, _ = op.root.Insert(selector, value)
			}

			if !yield(unsafe.Pointer(&op.root)) {
				return
			}
		}
	}
}
