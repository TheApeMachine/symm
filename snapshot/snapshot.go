/*
Package snapshot provides a lock-free, reader-wait-free state container for
per-symbol signal state.

Motivation. Every signal package keeps a *symbolState that is mutated by
multiple goroutines (tick, trade, book, feedback, publish) and read by a
publisher that ranges a sync.Map across all symbols. A sync.RWMutex per
symbol would serialize the per-symbol updaters; a single global mutex
would serialize everything. Both lose the inter-symbol parallelism that
makes this system fast.

The Cell type below holds an atomic.Pointer to an immutable T. Writers do
the read-modify-write cycle locally — load → mutate copy → CAS — so two
concurrent writers either commit independently or one retries against the
other's snapshot. Readers do a single atomic.Load and read freely from the
immutable copy. There is no mutex on the hot read path and no contention
across symbols.

For state that is dominated by long append-only series (ticks, samples,
ring buffers), the immutable T should hold the series as a slice whose
backing array is treated as immutable from the reader's perspective.
Writers that append within the existing capacity must still allocate a new
backing array to avoid the slice-header race; the bundled Append helper
performs that copy and is what every migrated signal calls.

This pattern is RCU-equivalent. It trades a small per-update allocation
(the cloned T) for genuinely concurrent reads, no head-of-line blocking
on a single writer, and a snapshot the publisher can hold for its entire
range without any other goroutine interfering with it.
*/
package snapshot

import "sync/atomic"

/*
Cell wraps an atomic.Pointer to an immutable T and exposes the standard
load / mutate primitives. Zero value is ready to use; Load on an
uninitialized Cell returns the zero value of T via the second return.
*/
type Cell[T any] struct {
	ptr atomic.Pointer[T]
}

/*
New returns a Cell containing the given value.
*/
func New[T any](value T) *Cell[T] {
	cell := &Cell[T]{}
	cell.ptr.Store(&value)

	return cell
}

/*
Load returns the current snapshot. The returned pointer is safe to read
indefinitely without coordination. ok is false when no value has been
stored yet.
*/
func (cell *Cell[T]) Load() (*T, bool) {
	if cell == nil {
		return nil, false
	}

	snap := cell.ptr.Load()

	return snap, snap != nil
}

/*
Mutate runs fn against a copy of the current snapshot and atomically
swaps it in. fn is called once on the no-contention path; concurrent
writers retry with the updated snapshot. The provided pointer points at a
private copy fn can freely mutate. Returning false from fn aborts the
update; the Cell is left untouched.

Mutate is the only correct way to update a Cell from a writer that wants
its update to compose with other writers.
*/
func (cell *Cell[T]) Mutate(fn func(*T) bool) {
	if cell == nil || fn == nil {
		return
	}

	for {
		prev := cell.ptr.Load()
		var next T

		if prev != nil {
			next = *prev
		}

		if !fn(&next) {
			return
		}

		if cell.ptr.CompareAndSwap(prev, &next) {
			return
		}
	}
}

/*
Append returns a new slice with value appended, allocating a fresh backing
array whenever the source already lives inside one. This is what every
writer that grows a series uses to avoid mutating a slice a reader may
still be iterating. cap is the soft maximum length; entries past cap are
trimmed from the front.
*/
func Append[T any](source []T, value T, cap int) []T {
	if cap > 0 && len(source) >= cap {
		next := make([]T, 0, cap)
		next = append(next, source[len(source)-cap+1:]...)
		next = append(next, value)

		return next
	}

	if cap <= 0 {
		cap = len(source) + 1
	}

	next := make([]T, 0, cap)
	next = append(next, source...)
	next = append(next, value)

	return next
}
