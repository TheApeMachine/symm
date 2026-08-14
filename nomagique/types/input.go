package types

/*
Input is anything a later stage can consume. A primitive, equation, metric, or
algorithm implements Input so its result can be wired forward without copying
it into a new bag of fields.
*/
type Input[T comparable] interface {
	IO[T]
}

func NewInput[T comparable]() Input[T] {
	return &InputValue[T]{}
}

func NewInputs[T comparable](count int) []Input[T] {
	collected := make([]Input[T], count)

	for index := range collected {
		collected[index] = NewInput[T]()
	}

	return collected
}

type InputValue[T comparable] struct {
	Value Value[T] `json:"value"`
}

func (input *InputValue[T]) Read() Output[T] {
	return input
}

func (input *InputValue[T]) Write(next Input[T]) {
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
