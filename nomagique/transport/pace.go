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

func NewPace[T any](delay types.Integer) Pace[T] {
	return func(in T) T {
		if delay != nil {
			if ms := delay(in); ms > 0 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}

		return in
	}
}
