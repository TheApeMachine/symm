package calculus

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Accumulate maintains a running cumulative sum of inputs written to it.
*/
type Accumulate struct {
	delta types.Input[float64]
	value types.Value[float64]
	total float64
	err   error
}

var _ types.IO[float64] = (*Accumulate)(nil)

/*
NewAccumulate creates an Accumulate primitive starting at initial value.
*/
func NewAccumulate(initial float64) *Accumulate {
	return &Accumulate{
		delta: types.NewInput[float64](),
		total: initial,
	}
}

/*
Write stages the increment to accumulate.
*/
func (accumulate *Accumulate) Write(input types.IO[float64]) {
	if input == nil {
		accumulate.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"accumulate: input is nil",
			nil,
		))

		return
	}

	accumulate.delta.Write(input)
	accumulate.err = nil
}

/*
Read adds the staged increment to the total and returns the updated accumulator.
*/
func (accumulate *Accumulate) Read() types.IO[float64] {
	increment := accumulate.delta.Read()

	if increment.Error() != "" {
		accumulate.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			increment.Error(),
			nil,
		))

		return accumulate
	}

	accumulate.total += increment.Project().Read()
	accumulate.value = accumulate.value.Write(accumulate.total)
	accumulate.err = nil

	return accumulate
}

/*
Project returns the current cumulative total.
*/
func (accumulate *Accumulate) Project() types.Value[float64] {
	return accumulate.value
}

/*
Error returns any execution error.
*/
func (accumulate *Accumulate) Error() string {
	if accumulate.err == nil {
		return ""
	}

	return accumulate.err.Error()
}

/*
Close resets the accumulator.
*/
func (accumulate *Accumulate) Close() error {
	if err := accumulate.delta.Close(); err != nil {
		return err
	}

	accumulate.total = 0
	accumulate.value = types.Value[float64]{}
	accumulate.err = nil

	return nil
}
