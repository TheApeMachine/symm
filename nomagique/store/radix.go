package store

import (
	"bytes"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Radix associates values with ordered keys, so a key can be asked about by its
opening and not only in full. Everything stored beneath an opening sits together
and is reached by one walk, which is what lets a longer sequence be answered for
by a shorter one that was actually observed.

It never mutates its configured source. Retaining successive trees is explicit
feedback through Retained, exactly as it is for KV: the store reads its state
and hands back the next one, and the composition decides whether that is kept.
*/
type Radix[T iradix.Tree[any]] struct {
	core.PrimitiveError
	current core.Primitive
}

func NewRadix[T iradix.Tree[any]](state core.Primitive) *Radix[T] {
	return &Radix[T]{
		current: state,
	}
}

func (radix *Radix[T]) Next(in core.Primitive) core.Primitive {
	return core.Yield(
		radix.current,
		in,
		func(held *iradix.Tree[[]byte], arriving map[string][]byte) *iradix.Tree[[]byte] {
			if held == nil {
				held = iradix.New[[]byte]()
			}
			selector, selecting := arriving["selector"]
			data, writing := arriving["data"]

			// A selector with nothing to write is a question. What was asked
			// for sits beneath it, and whoever asked reads it out of the store
			// they are handed back, inside their own fold.
			if !writing {
				return held
			}

			// Data with no selector is not addressed anywhere, and writing it
			// somewhere chosen here would be inventing a key nobody asked for.
			if !selecting {
				radix.Error(core.ErrShape)

				return held
			}
			written, _, _ := held.Insert(selector, bytes.Clone(data))

			return written
		},
		radix,
	)
}

func (radix *Radix[T]) Read() any { return radix.current }
