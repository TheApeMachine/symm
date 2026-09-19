package temporal

import "github.com/theapemachine/symm/nomagique/types"

/*
NewDelay creates a stateful closure that acts as a delay line (shift register).
It queues inputs and yields the value that arrived `horizon` steps ago.
Before the horizon is reached, it yields the zero value of T.
*/
type Delay[T any] types.Value[T, T]
func NewDelay[T any](horizon int) Delay[T] {
	if horizon < 1 {
		horizon = 1
	}
	
	buffer := make([]T, horizon)
	var count int

	return func(in T) T {
		var out T
		if count >= horizon {
			out = buffer[count%horizon]
		}
		
		buffer[count%horizon] = in
		count++
		
		return out
	}
}
