package transport

import (
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Pace introduces a deterministic rate-limiting or delay window between operations.
No structs, pure Value closure.
*/
type Pace[T any] types.Value[T, T]

func NewPace[T any](delay time.Duration) Pace[T] {
	return func(in T) T {
		if delay > 0 {
			time.Sleep(delay)
		}

		return in
	}
}
