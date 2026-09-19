package transport

import (
	"sync"

	"github.com/theapemachine/symm/nomagique/types"
)

type Tee[T, U any] types.Value[T, T]
/*
NewTee creates a Value closure that taps input to offramp closures,
then returns the original input unchanged.
No structs, pure Value closure.
*/
func NewTee[T, U any](offramps ...types.Value[T, U]) Tee[T, U] {
	return func(in T) T {
		for _, offramp := range offramps {
			if offramp != nil {
				offramp(in)
			}
		}
		return in
	}
}

type Discard[T any] types.Value[T, struct{}]
/*
NewDiscard creates a Value closure that drops input and returns struct{}.
*/
func NewDiscard[T any]() Discard[T] {
	return func(T) struct{} {
		return struct{}{}
	}
}

type Fan[T, U any] types.Value[T, []U]
/*
NewFan creates a Value closure that broadcasts input across multiple branches,
collecting all results into a typed slice []U.
*/
func NewFan[T, U any](branches ...types.Value[T, U]) Fan[T, U] {
	return func(in T) []U {
		out := make([]U, len(branches))

		for i, b := range branches {
			if b != nil {
				out[i] = b(in)
			}
		}

		return out
	}
}

type Parallel[T, U any] types.Value[[]T, []U]
/*
NewParallel creates a Value closure that processes items concurrently across workers.
*/
func NewParallel[T, U any](task types.Value[T, U]) Parallel[T, U] {
	return func(items []T) []U {
		if len(items) == 0 {
			return nil
		}

		out := make([]U, len(items))
		var wg sync.WaitGroup
		wg.Add(len(items))

		for i := range items {
			go func(idx int) {
				defer wg.Done()
				out[idx] = task(items[idx])
			}(i)
		}

		wg.Wait()
		return out
	}
}

type Action uint8

const (
	PEEK Action = iota
	POKE
	REGISTER
)

// Message guarantees the types of the payload and the primitive.
type Message[T, U any] func() (Action, T, types.Value[T, U])

func NewMessage[T, U any](action Action, value T, primitive types.Value[T, U]) Message[T, U] {
	return func() (Action, T, types.Value[T, U]) {
		return action, value, primitive
	}
}

// IO is a one-way communication primitive.
type IO[T, U any] types.Value[T, U]

func NewIO[T, U any](handler types.Value[T, U]) IO[T, U] {
	return IO[T, U](handler)
}

// Conn is a bi-directional communication primitive that composes two IOs.
type Conn[T, U any] func(Action, T) U

func NewConn[T, U any](rx IO[T, U], tx IO[T, U]) Conn[T, U] {
	return func(action Action, in T) U {
		if action == PEEK {
			return rx(in)
		}
		return tx(in)
	}
}
