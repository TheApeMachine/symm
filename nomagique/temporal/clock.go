package temporal

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Clock is a timing primitive. Age is how far the walk has gone; span is the
characteristic duration. Read emits progress = age/span. A non-positive span
is an error, not a guessed window.
*/
type Clock struct {
	age   types.Input[float64]
	span  types.Input[float64]
	value types.Value[float64]
	err   error
}

/*
NewClock stages age and span in the same unit.
*/
func NewClock(age types.Input[float64], span types.Input[float64]) *Clock {
	return &Clock{
		age:  age,
		span: span,
	}
}

/*
Write stages a new age from the source. Span stays the characteristic duration
this clock was constructed with.
*/
func (clock *Clock) Write(input types.Input[float64]) {
	clock.age.Write(input)
	clock.err = nil
}

/*
Read executes progress and returns the clock as output.
*/
func (clock *Clock) Read() types.Output[float64] {
	age := clock.age.Read()
	span := clock.span.Read()

	if age.Error() != "" {
		clock.err = errnie.Error(errnie.Err(
			errnie.Validation,
			age.Error(),
			nil,
		))

		return clock
	}

	if span.Error() != "" {
		clock.err = errnie.Error(errnie.Err(
			errnie.Validation,
			span.Error(),
			nil,
		))

		return clock
	}

	if span.Project().Read() <= 0 {
		clock.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"clock: positive span required",
			nil,
		))

		return clock
	}

	if age.Project().Read() < 0 {
		clock.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"clock: age cannot be negative",
			nil,
		))

		return clock
	}

	clock.value = clock.value.Write(age.Project().Read() / span.Project().Read())
	clock.err = nil

	return clock
}

/*
Project is the last computed progress.
*/
func (clock *Clock) Project() types.Value[float64] {
	return clock.value
}

/*
Error reports a staging or execution failure.
*/
func (clock *Clock) Error() string {
	if clock.err == nil {
		return ""
	}

	return clock.err.Error()
}

/*
Close releases staged state.
*/
func (clock *Clock) Close() error {
	if err := clock.age.Close(); err != nil {
		return err
	}

	if err := clock.span.Close(); err != nil {
		return err
	}

	clock.value = types.Value[float64]{}
	clock.err = nil

	return nil
}
