package temporal

import "github.com/theapemachine/symm/nomagique/types"

/*
NewDelay creates a stateful closure that acts as a delay line (shift register).
It queues inputs and yields the value that arrived `horizon` steps ago.
Before the horizon is reached, it yields the zero value of T.
*/
type Delay[T any] types.Value[T, T]
func NewDelay[T any](horizon types.Integer) Delay[T] {
	var buffer []T
	var count int

	return func(in T) T {
		h := 1
		if horizon != nil {
			if evaluated := horizon(in); evaluated > 0 {
				h = evaluated
			}
		}

		if len(buffer) != h {
			newBuf := make([]T, h)
			copy(newBuf, buffer)
			buffer = newBuf
		}

		var out T
		if count >= h {
			out = buffer[count%h]
		}

		buffer[count%h] = in
		count++

		return out
	}
}
