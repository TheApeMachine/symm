package equation

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*Squash maps positive evidence through an empirical positive scale.
Its map carries "value" and "scale", producing "result".*/
type Squash struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Squash)(nil)

/*NewSquash returns a Squash excitation primitive.*/
func NewSquash(initial types.Input[types.Map[string, types.Value[float64]]]) *Squash {
	return &Squash{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*Write stages the squash map.*/
func (squash *Squash) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		squash.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"squash: input is nil",
			nil,
		))

		return
	}

	squash.next.Write(input)
	squash.err = nil
}

/*Read computes the squash result = value / (scale + value).*/
func (squash *Squash) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := squash.next.Read()

	if in.Error() != "" {
		squash.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return squash.next
	}

	mapping := in.Project().Read()
	valueVal, hasValue := mapping.Get("value")
	scaleVal, hasScale := mapping.Get("scale")

	if !hasValue || !hasScale {
		squash.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"squash: missing value or scale",
			nil,
		))

		return squash.next
	}

	value := valueVal.Read()
	scale := scaleVal.Read()

	if value <= 0 || scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		result := 0.0
		mapping.Put("result", types.NewValue(result))
		squash.next.Write(types.NewInput(types.NewValue(mapping)))
		squash.err = nil

		return squash.next
	}

	result := value / (scale + value)
	mapping.Put("result", types.NewValue(result))

	squash.next.Write(types.NewInput(types.NewValue(mapping)))
	squash.err = nil

	return squash.next
}

/*Project returns the current projected map.*/
func (squash *Squash) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return squash.next.Project()
}

/*Error reports any execution error.*/
func (squash *Squash) Error() string {
	if squash.err != nil {
		return squash.err.Error()
	}

	return squash.next.Error()
}

/*Close releases internal state.*/
func (squash *Squash) Close() error {
	if err := squash.initial.Close(); err != nil {
		return err
	}

	if err := squash.next.Close(); err != nil {
		return err
	}

	squash.err = nil

	return nil
}