package store

import (
	"bytes"
	"iter"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Radix associates values with ordered keys. It never mutates its configured
source: each arrival is applied to a tree and the next tree is handed over.
*/
type Radix struct {
	core.Base[map[string][]byte, *iradix.Tree[[]byte]]
}

/*
NewRadix begins on an empty tree, or continues one already held.

A composition declares its stages before it has a memory to put in them, so the
tree is optional: given none, the store starts empty and grows as it is written
to, which is what a fresh learner is.
*/
func NewRadix(current ...*iradix.Tree[[]byte]) *Radix {
	op := &Radix{}
	held := iradix.New[[]byte]()

	if len(current) > 0 && current[0] != nil {
		held = current[0]
	}
	op.Carrier(held)

	return op
}

func (op *Radix) Next(
	in iter.Seq[core.Primitive[map[string][]byte, map[string][]byte]],
) iter.Seq[core.Primitive[*iradix.Tree[[]byte], *iradix.Tree[[]byte]]] {
	return func(yield func(core.Primitive[*iradix.Tree[[]byte], *iradix.Tree[[]byte]]) bool) {
		for arriving := range in {
			held := op.Read()

			if held == nil {
				held = iradix.New[[]byte]()
			}

			fields := arriving.Read()
			selector, selecting := fields["selector"]
			data, writing := fields["data"]

			if !writing {
				if !yield(op.Carrier(held)) {
					return
				}

				continue
			}

			if !selecting {
				op.Error(core.ErrShape)

				if !yield(op.Carrier(held)) {
					return
				}

				continue
			}

			written, _, _ := held.Insert(selector, bytes.Clone(data))

			if !yield(op.Carrier(written)) {
				return
			}
		}
	}
}
