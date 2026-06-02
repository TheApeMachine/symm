package snapshot

import "sync"

/*
SlicePool reuses slice backings for append-heavy hot paths so high-frequency
writers do not allocate a fresh backing array on every copy-on-write update.
*/
type SlicePool[T any] struct {
	pool       sync.Pool
	defaultCap int
}

func NewSlicePool[T any](defaultCap int) *SlicePool[T] {
	capacity := defaultCap

	if capacity <= 0 {
		capacity = 16
	}

	return &SlicePool[T]{
		defaultCap: capacity,
		pool: sync.Pool{
			New: func() any {
				buffer := make([]T, 0, capacity)

				return &buffer
			},
		},
	}
}

func (slicePool *SlicePool[T]) Acquire() []T {
	buffer := slicePool.pool.Get().(*[]T)

	return (*buffer)[:0]
}

func (slicePool *SlicePool[T]) Release(buffer []T) {
	if buffer == nil {
		return
	}

	if cap(buffer) < slicePool.defaultCap/2 {
		return
	}

	copied := buffer[:0]
	slicePool.pool.Put(&copied)
}

/*
AppendWithPool appends value using a pooled backing array when cap is known.
When source is non-empty the previous backing is returned to the pool.
*/
func AppendWithPool[T any](
	slicePool *SlicePool[T],
	source []T,
	value T,
	maxLen int,
) []T {
	next := slicePool.Acquire()

	if maxLen > 0 && len(source) >= maxLen {
		next = append(next, source[len(source)-maxLen+1:]...)
		next = append(next, value)

		slicePool.Release(source)

		return next
	}

	capacity := maxLen

	if capacity <= 0 {
		capacity = len(source) + 1
	}

	if cap(next) < capacity {
		next = make([]T, 0, capacity)
	}

	next = append(next, source...)
	next = append(next, value)

	slicePool.Release(source)

	return next
}
