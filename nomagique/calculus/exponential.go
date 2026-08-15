package calculus

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Exponential transforms a numeric value exponentially, each time
Read is called. An initial value can be provided, or set via Write.
*/
type Exponential struct {
	initial  types.Input[float64]
	next	 types.Input[float64]
	err      error
}

/*
NewExponential returns an unstaged exponential shape.
*/
func NewExponential(initial types.Input[float64]) *Exponential {
	return &Exponential{
		initial: initial,
		next:    types.NewInput[float64](initial.Project()),
	}
}

/*
Write stages progress from the source.
*/
func (shape *Exponential) Write(input types.Input[float64]) {
	shape.progress.Write(input)
	shape.err = nil
}

/*
Read executes the remaining fraction and returns the shape as output.
*/
func (shape *Exponential) Read() types.Output[float64] {
	progress := shape.progress.Read()

	if progress.Error() != "" {
		shape.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			progress.Error(),
			nil,
		))

		return shape
	}

	shape.value = shape.value.Write(math.Exp(-progress.Project().Read()))
	shape.err = nil

	return shape
}

/*
Project is the last remaining fraction.
*/
func (shape *Exponential) Project() types.Value[float64] {
	return shape.value
}

/*
Error reports a staging or execution failure.
*/
func (shape *Exponential) Error() string {
	if shape.err == nil {
		return ""
	}

	return shape.err.Error()
}

/*
Close releases staged state.
*/
func (shape *Exponential) Close() error {
	if err := shape.progress.Close(); err != nil {
		return err
	}

	shape.value = types.Value[float64]{}
	shape.err = nil

	return nil
}
