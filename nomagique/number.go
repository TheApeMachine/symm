package nomagique

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewNumber instantiates a nomagique pipeline composer with the given stages.
It chains stages of types.Value closures sequentially.

nomagique.Number
no, magic, number
*/
type Number[T any] types.Value[T, T]

func NewNumber[T any](stages ...types.Value[T, T]) types.Value[T, T] {
	return func(in T) T {
		curr := in

		for _, stage := range stages {
			if stage != nil {
				curr = stage(curr)
			}
		}

		return curr
	}
}
