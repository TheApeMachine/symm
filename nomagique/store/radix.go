package store

import (
	"bytes"
	"iter"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Radix owns immutable addressed values. One writer publishes new roots; readers
borrow values from the root they loaded. Read misses yield nothing. Identify
inserts its explicit initial payload only when the key is absent. Payloads must
be values without mutable aliases: the store copies T, not an object graph.
Writes retain their own key bytes; reads borrow the caller's address.
*/
type Radix[T any] struct {
	*core.PrimitiveError
	root atomic.Pointer[iradix.Tree[T]]
}

func NewRadix[T any]() *Radix[T] {
	radix := &Radix[T]{PrimitiveError: core.NewPrimitiveError()}
	radix.root.Store(iradix.New[T]())
	return radix
}

func (radix *Radix[T]) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range input {
			query := (*Query[*[]byte, T])(arriving)
			address := query.Identity()

			if address == nil || len(*address) == 0 {
				radix.Error(core.ErrShape)
				return
			}

			root := radix.root.Load()
			value, found := root.Get(*address)

			switch query.Action {
			case core.Read:
				if found && !yield(unsafe.Pointer(&value)) {
					return
				}

			case core.Identify:
				if found {
					if !yield(unsafe.Pointer(&value)) {
						return
					}
					continue
				}

				if query.Payload != nil {
					for ptr := range query.Payload {
						value = *(*T)(ptr)
						root = radix.root.Load()
						updated, _, _ := root.Insert(bytes.Clone(*address), value)
						radix.root.Store(updated)

						if !yield(unsafe.Pointer(&value)) {
							return
						}
					}
				}

			case core.Write:
				if query.Payload != nil {
					for ptr := range query.Payload {
						value = *(*T)(ptr)
						root = radix.root.Load()
						updated, _, _ := root.Insert(bytes.Clone(*address), value)
						radix.root.Store(updated)

						if !yield(unsafe.Pointer(&value)) {
							return
						}
					}
				}

			default:
				radix.Error(core.ErrShape)
				return
			}
		}
	}
}
