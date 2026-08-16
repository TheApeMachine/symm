package temporal

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Clock calculates temporal progress = age / span.
Its map carries "age", "span", and the computed "progress".
*/
type Clock struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Clock)(nil)

/*
NewClock instantiates a Clock primitive.
*/
func NewClock(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Clock {
	return &Clock{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*
Write stages the clock parameter map.
*/
func (clock *Clock) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		clock.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"clock: input is nil",
			nil,
		))

		return
	}

	clock.next.Write(input)
	clock.err = nil
}

/*
Read computes progress = age / span.
*/
func (clock *Clock) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := clock.next.Read()

	if in.Error() != "" {
		clock.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return clock.next
	}

	mapping := in.Project().Read()
	ageVal, hasAge := mapping.Get("age")
	spanVal, hasSpan := mapping.Get("span")

	if !hasAge || !hasSpan {
		clock.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"clock: missing age or span",
			nil,
		))

		return clock.next
	}

	age := ageVal.Read()
	span := spanVal.Read()

	if span <= 0 {
		clock.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"clock: positive span required",
			nil,
		))

		return clock.next
	}

	progress := age / span
	mapping.Put("progress", types.NewValue(progress))

	clock.next.Write(types.NewInput(types.NewValue(mapping)))
	clock.err = nil

	return clock.next
}

/*
Project returns the current projected map.
*/
func (clock *Clock) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return clock.next.Project()
}

/*
Error reports any execution error.
*/
func (clock *Clock) Error() string {
	if clock.err != nil {
		return clock.err.Error()
	}

	return clock.next.Error()
}

/*
Close releases internal state.
*/
func (clock *Clock) Close() error {
	if err := clock.initial.Close(); err != nil {
		return err
	}

	if err := clock.next.Close(); err != nil {
		return err
	}

	clock.err = nil

	return nil
}
