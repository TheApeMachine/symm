package nomagique

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Iterate wraps a Value closure and executes it repeatedly for a given number of steps,
feeding the output back as the next input.
*/
func Iterate[T any](steps types.Integer, stage types.Value[T, T]) types.Value[T, T] {
	return func(in T) T {
		curr := in
		n := 0
		if steps != nil {
			n = steps(in)
		}
		for range n {
			if stage != nil {
				curr = stage(curr)
			}
		}
		return curr
	}
}

/*
IterateUntil executes a closure repeatedly up to maxSteps, but stops early
if the condition evaluates to true.
*/
func IterateUntil[T any](maxSteps types.Integer, condition types.Value[T, bool], stage types.Value[T, T]) types.Value[T, T] {
	return func(in T) T {
		curr := in
		n := 0
		if maxSteps != nil {
			n = maxSteps(in)
		}
		for range n {
			if stage != nil {
				curr = stage(curr)
			}
			if condition != nil && condition(curr) {
				break
			}
		}
		return curr
	}
}
