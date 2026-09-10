package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Values lifts a fixed list of payloads into a run. Each payload travels as its
own Primitive; the list is not itself a Primitive.
*/
func Values[T any](values ...T) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for _, value := range values {
			carrier := &core.Carrier[T]{}

			if !yield(carrier.Carrier(value)) {
				return
			}
		}
	}
}

/*
One presents a single Primitive as a run.
*/
func One[T any](value core.Primitive[T, T]) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		yield(value)
	}
}
