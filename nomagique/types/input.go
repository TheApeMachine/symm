package types

import (
	"iter"
)

/*
Input is anything a later stage can consume. A primitive, equation, metric, or
algorithm implements Input so its result can be wired forward without copying
it into a new bag of fields.
*/
type Input[T comparable] interface {
	IO[T]
}

func NewInput[T comparable](values ...Value[T]) Input[T] {
	if len(values) > 0 {
		return &InputValue[T]{
			Value: values[0],
		}
	}

	return &InputValue[T]{}
}

/*
NewErrorInput returns a staged value carrying an execution failure. The current
payload is retained as Zero so stateful primitives can reject a candidate
without discarding their last committed state.
*/
func NewErrorInput[T comparable](current T, err error) Input[T] {
	return NewInput(Value[T]{
		Zero:    current,
		Initial: current,
		Ready:   true,
		Err:     err,
	})
}

func NewInputs[T comparable](values ...Value[T]) iter.Seq[Input[T]] {
	return func(yield func(Input[T]) bool) {
		for _, value := range values {
			if !yield(NewInput(value)) {
				return
			}
		}
	}
}

type InputValue[T comparable] struct {
	Value Value[T] `json:"value"`
}

func (input *InputValue[T]) Read() IO[T] {
	return input
}

func (input *InputValue[T]) Write(next IO[T]) {
	input.Value = input.Value.Write(next.Project().Read())
}

func (input *InputValue[T]) Project() Value[T] {
	return input.Value
}

func (input *InputValue[T]) Error() string {
	return input.Value.Error()
}

func (input *InputValue[T]) Close() error {
	return input.Value.Close()
}
