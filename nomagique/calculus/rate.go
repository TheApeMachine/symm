package calculus

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Rate calculates empirical arrival rate = count / duration.
Its map carries "count", "duration", and "rate".
*/
type Rate struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Rate)(nil)

/*
NewRate creates a Rate primitive.
*/
func NewRate(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Rate {
	return &Rate{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*
Write stages the rate calculation map.
*/
func (rate *Rate) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		rate.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"rate: input is nil",
			nil,
		))

		return
	}

	rate.next.Write(input)
	rate.err = nil
}

/*
Read calculates arrival rate = count / duration.
*/
func (rate *Rate) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := rate.next.Read()

	if in.Error() != "" {
		rate.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return rate.next
	}

	mapping := in.Project().Read()
	countVal, hasCount := mapping.Get("count")
	durVal, hasDur := mapping.Get("duration")

	if !hasCount || !hasDur {
		rate.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"rate: missing count or duration",
			nil,
		))

		return rate.next
	}

	duration := durVal.Read()

	if duration <= 0 {
		rate.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"rate: duration must be positive",
			nil,
		))

		return rate.next
	}

	result := countVal.Read() / duration
	mapping.Put("rate", types.NewValue(result))

	rate.next.Write(types.NewInput(types.NewValue(mapping)))
	rate.err = nil

	return rate.next
}

/*
Project returns the current projected map.
*/
func (rate *Rate) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return rate.next.Project()
}

/*
Error reports any execution error.
*/
func (rate *Rate) Error() string {
	if rate.err != nil {
		return rate.err.Error()
	}

	return rate.next.Error()
}

/*
Close resets the primitive state.
*/
func (rate *Rate) Close() error {
	if err := rate.initial.Close(); err != nil {
		return err
	}

	if err := rate.next.Close(); err != nil {
		return err
	}

	rate.err = nil

	return nil
}
