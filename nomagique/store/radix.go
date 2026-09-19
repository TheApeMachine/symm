package store

import (
	"bytes"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Action specifies the operation performed on an addressable store.
*/
type Action int

const (
	Read Action = iota
	Write
	Identify
)

/*
RadixCommandData carries the address, payload, and operation for Radix storage.
*/
type RadixCommandData[T any] struct {
	Key    []byte
	Value  T
	Action Action
}

/*
NewRadixCommand builds a command carrier closure for pipelines.
No structs, pure Value closure.
*/
type RadixCommand[T any] types.Value[T, RadixCommandData[T]]
func NewRadixCommand[T any](key []byte, action Action) RadixCommand[T] {
	return func(val T) RadixCommandData[T] {
		return RadixCommandData[T]{
			Key:    key,
			Value:  val,
			Action: action,
		}
	}
}

/*
NewRadix owns immutable addressed values. One writer publishes new roots; readers
borrow values from the root they loaded. Read misses yield nil. Identify
inserts its explicit initial payload only when the key is absent. Payloads must
be values without mutable aliases: the store copies T, not an object graph.
Writes retain their own key bytes; reads borrow the caller's address.
No structs, pure Value closure.
*/
type Radix[T any] types.Value[RadixCommandData[T], *T]
func NewRadix[T any]() Radix[T] {
	var root atomic.Pointer[iradix.Tree[T]]
	root.Store(iradix.New[T]())

	return func(cmd RadixCommandData[T]) *T {
		if len(cmd.Key) == 0 {
			return nil
		}

		current := root.Load()
		val, found := current.Get(cmd.Key)

		switch cmd.Action {
		case Read:
			if !found {
				return nil
			}
			out := val
			return &out

		case Identify:
			if found {
				out := val
				return &out
			}
			updated, _, _ := current.Insert(bytes.Clone(cmd.Key), cmd.Value)
			root.Store(updated)
			res, _ := updated.Get(cmd.Key)
			return &res

		case Write:
			updated, _, _ := current.Insert(bytes.Clone(cmd.Key), cmd.Value)
			root.Store(updated)
			res, _ := updated.Get(cmd.Key)
			return &res

		default:
			return nil
		}
	}
}
