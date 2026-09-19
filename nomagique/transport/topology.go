package transport

import (
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Fork evaluates multiple branches for an input, collecting the results into a slice.
No structs, pure Value closure.
*/
type Fork[T, U any] types.Value[T, []U]

func NewFork[T, U any](branches ...types.Value[T, U]) Fork[T, U] {
	return func(in T) []U {
		results := make([]U, len(branches))

		for i, branch := range branches {
			if branch == nil {
				continue
			}

			results[i] = branch(in)
		}

		return results
	}
}

/*
Join combines a slice of values into a single result using a combiner function.
*/
type Join[T, U any] types.Value[[]T, U]

func NewJoin[T, U any](combiner types.Value[[]T, U]) Join[T, U] {
	return func(items []T) U {
		if combiner == nil {
			var zero U
			return zero
		}
		return combiner(items)
	}
}

/*
Route sends an input to a specific handler selected by key from a router function.
*/
type Route[T, U any] types.Value[T, U]

func NewRoute[T, U any](router types.Value[T, string], routes map[string]types.Value[T, U]) Route[T, U] {
	return func(in T) U {
		if router == nil {
			var zero U
			return zero
		}
		key := router(in)
		handler, ok := routes[key]
		if !ok || handler == nil {
			var zero U
			return zero
		}

		return handler(in)
	}
}

/*
Gate evaluates a predicate and conditionally forwards the input, or returns a zero value.
*/
type Gate[T any] types.Value[T, *T]

func NewGate[T any](predicate types.Value[T, bool]) Gate[T] {
	return func(in T) *T {
		if predicate == nil || !predicate(in) {
			return nil
		}

		return &in
	}
}

/*
Broadcast forwards an input to multiple concurrent subscribers.
*/
type Broadcast[T any] types.Value[T, T]

func NewBroadcast[T any](subscribers ...types.Value[T, any]) Broadcast[T] {
	return func(in T) T {
		var wg sync.WaitGroup
		wg.Add(len(subscribers))

		for _, sub := range subscribers {
			if sub == nil {
				wg.Done()
				continue
			}

			go func(fn types.Value[T, any]) {
				defer wg.Done()
				fn(in)
			}(sub)
		}

		wg.Wait()
		return in
	}
}

/*
Collect aggregates items into a slice of a configured batch size.
*/
type Collect[T any] types.Value[T, []T]

func NewCollect[T any](batchSize types.Integer) Collect[T] {
	var buf []T

	return func(in T) []T {
		bs := 0
		if batchSize != nil {
			bs = batchSize(in)
		}
		if bs <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"collect: batch size must be positive",
				nil,
			))
			return nil
		}

		buf = append(buf, in)
		if len(buf) < bs {
			return nil
		}

		batch := make([]T, len(buf))
		copy(batch, buf)
		buf = buf[:0]
		return batch
	}
}
