package calculus

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Decay walks a value toward zero. With no shape that walk is linear. A shape
IO (Exponential) and a clock IO replace the glued exp(-βt) form.
*/
type Decay struct {
	clock types.IO[float64]
	shape types.IO[float64]
	start types.Input[float64]
	value types.Value[float64]
	err   error
}

var _ types.IO[float64] = (*Decay)(nil)

/*
NewDecay takes a clock and an optional shape. A nil shape is linear.
*/
func NewDecay(clock types.IO[float64], shape types.IO[float64]) *Decay {
	return &Decay{
		clock: clock,
		shape: shape,
		start: types.NewInput[float64](),
	}
}

/*
Write stages the starting level from the source.
*/
func (decay *Decay) Write(input types.Input[float64]) {
	decay.start.Write(input)
	decay.err = nil
}

/*
Read executes the remaining level and returns the decay as output.
*/
func (decay *Decay) Read() types.Output[float64] {
	if decay.clock == nil {
		decay.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"decay: clock required",
			nil,
		))

		return decay
	}

	start := decay.start.Read()

	if start.Error() != "" {
		decay.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			start.Error(),
			nil,
		))

		return decay
	}

	progress := decay.clock.Read()

	if progress.Error() != "" {
		decay.err = errnie.Error(errnie.Err(
			errnie.Validation,
			progress.Error(),
			nil,
		))

		return decay
	}

	remaining := 1 - progress.Project().Read()

	if remaining < 0 {
		remaining = 0
	}

	if decay.shape != nil {
		decay.shape.Write(progress)
		shaped := decay.shape.Read()

		if shaped.Error() != "" {
			decay.err = errnie.Error(errnie.Err(
				errnie.Validation,
				shaped.Error(),
				nil,
			))

			return decay
		}

		remaining = shaped.Project().Read()
	}

	decay.value = decay.value.Write(start.Project().Read() * remaining)
	decay.err = nil

	return decay
}

/*
Project is the last remaining level.
*/
func (decay *Decay) Project() types.Value[float64] {
	return decay.value
}

/*
Error reports a staging or execution failure.
*/
func (decay *Decay) Error() string {
	if decay.err == nil {
		return ""
	}

	return decay.err.Error()
}

/*
Close resets and closes owned modifiers.
*/
func (decay *Decay) Close() error {
	if decay.clock != nil {
		if err := decay.clock.Close(); err != nil {
			return err
		}
	}

	if decay.shape != nil {
		if err := decay.shape.Close(); err != nil {
			return err
		}
	}

	if err := decay.start.Close(); err != nil {
		return err
	}

	decay.value = types.Value[float64]{}
	decay.err = nil

	return nil
}
