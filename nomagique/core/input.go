package core

import (
	"iter"
	"unsafe"
)

/*
Input ...
The data value is consumed by the iterator.
Both origin and action are optional, depending on the Primtive
that will be receiving the Input.
*/
type Input[T, U any] struct {
	*PrimitiveError
	origin Identifiable[T]
	action *Action
	data   []U
}

/*
NewInput ...
*/
func NewInput[T, U any](
	origin Identifiable[T],
	action *Action,
	data ...U,
) *Input[T, U] {
	return &Input[T, U]{
		PrimitiveError: NewPrimitiveError(),
		origin:         origin,
		action:         action,
		data:           data,
	}
}

/*
Next ...
By supplying more input the data value can be replenished.
*/
func (input *Input[T, U]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in != nil {
			for i := range in {
				concrete := any(i).(U)
				input.data = append(input.data, concrete)
			}
		}

		var out U

		for range len(input.data) {
			out, input.data = input.data[0], input.data[1:]

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
