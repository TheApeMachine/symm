package nomagique

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Iterate wraps a Value closure and executes it repeatedly for a given number of steps,
feeding the output back as the next input.
*/
func Iterate[T any](steps int, stage types.Value[T, T]) types.Value[T, T] {
	return func(in T) T {
		curr := in
		for range steps {
			curr = stage(curr)
		}
		return curr
	}
}

/*
IterateUntil executes a closure repeatedly up to maxSteps, but stops early
if the condition evaluates to true.
*/
func IterateUntil[T any](maxSteps int, condition func(T) bool, stage types.Value[T, T]) types.Value[T, T] {
	return func(in T) T {
		curr := in
		for range maxSteps {
			curr = stage(curr)
			if condition(curr) {
				break
			}
		}
		return curr
	}
}
