package types

/*
Output is anything a stage produces. It embeds Input so a produced value is
already a legal input to the next primitive, equation, or algorithm.
*/
type Output[T comparable] interface {
	IO[T]
}

func NewOutput[T comparable]() Output[T] {
	return &OutputValue[T]{}
}

type OutputValue[T comparable] struct {
	Value Value[T] `json:"value"`
}

func (output *OutputValue[T]) Read() IO[T] {
	return output
}

func (output *OutputValue[T]) Write(next IO[T]) {
	output.Value = output.Value.Write(next.Project().Read())
}

func (output *OutputValue[T]) Project() Value[T] {
	return output.Value
}

func (output *OutputValue[T]) Error() string {
	return output.Value.Error()
}

func (output *OutputValue[T]) Close() error {
	return output.Value.Close()
}
