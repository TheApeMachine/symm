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

func NewRadix(current *iradix.Tree[[]byte]) *Radix {
	op := &Radix{}
	op.Carrier(current)
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
