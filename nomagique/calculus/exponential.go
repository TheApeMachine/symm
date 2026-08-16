package calculus

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Exponential transforms an input parameter exponentially: e^(-x).
*/
type Exponential struct {
	initial types.Input[float64]
	next    types.Input[float64]
	err     error
}

var _ types.IO[float64] = (*Exponential)(nil)

/*
NewExponential returns an Exponential shape primitive.
*/
func NewExponential(initial types.Input[float64]) *Exponential {
	return &Exponential{
		initial: initial,
		next:    types.NewInput[float64](),
	}
}

/*
Write stages the progress value.
*/
func (exponential *Exponential) Write(input types.IO[float64]) {
	if input == nil {
		exponential.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"exponential: input is nil",
			nil,
		))

		return
	}

	exponential.next.Write(input)
	exponential.err = nil
}

/*
Read computes e^(-progress) and returns the output.
*/
func (exponential *Exponential) Read() types.IO[float64] {
	in := exponential.next.Read()

	if in.Error() != "" {
		exponential.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return exponential.next
	}

	val := math.Exp(-in.Project().Read())
	exponential.next.Write(types.NewInput(types.NewValue(val)))
	exponential.err = nil

	return exponential.next
}

/*
Project returns the last computed exponential value.
*/
func (exponential *Exponential) Project() types.Value[float64] {
	return exponential.next.Project()
}

/*
Error reports an execution or staging failure.
*/
func (exponential *Exponential) Error() string {
	if exponential.err != nil {
		return exponential.err.Error()
	}

	return exponential.next.Error()
}

/*
Close resets the staged state.
*/
func (exponential *Exponential) Close() error {
	if err := exponential.initial.Close(); err != nil {
		return err
	}

	if err := exponential.next.Close(); err != nil {
		return err
	}

	exponential.err = nil

	return nil
}
