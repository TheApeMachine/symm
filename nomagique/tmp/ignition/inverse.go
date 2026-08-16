package equation

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*Inverse maps counter-evidence through an empirical scale. Measured
absence is complete quiet and therefore needs no positive-event scale.
Its map carries "value" and "scale", producing "result".*/
type Inverse struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Inverse)(nil)

/*NewInverse returns an Inverse excitation primitive.*/
func NewInverse(initial types.Input[types.Map[string, types.Value[float64]]]) *Inverse {
	return &Inverse{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*Write stages the inverse map.*/
func (inv *Inverse) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		inv.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"inverse: input is nil",
			nil,
		))

		return
	}

	inv.next.Write(input)
	inv.err = nil
}

/*Read computes the inverse result = scale / (scale + value).*/
func (inv *Inverse) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := inv.next.Read()

	if in.Error() != "" {
		inv.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return inv.next
	}

	mapping := in.Project().Read()
	valueVal, hasValue := mapping.Get("value")
	scaleVal, hasScale := mapping.Get("scale")

	if !hasValue || !hasScale {
		inv.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"inverse: missing value or scale",
			nil,
		))

		return inv.next
	}

	value := valueVal.Read()
	scale := scaleVal.Read()

	if value < 0 {
		inv.err = nil
		inv.next.Write(types.NewInput(types.NewValue(mapping)))
		return inv.next
	}

	if value == 0 {
		mapping.Put("result", types.NewValue(1.0))
		inv.next.Write(types.NewInput(types.NewValue(mapping)))
		inv.err = nil

		return inv.next
	}

	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		result := 0.0
		mapping.Put("result", types.NewValue(result))
		inv.next.Write(types.NewInput(types.NewValue(mapping)))
		inv.err = nil

		return inv.next
	}

	result := scale / (scale + value)
	mapping.Put("result", types.NewValue(result))

	inv.next.Write(types.NewInput(types.NewValue(mapping)))
	inv.err = nil

	return inv.next
}

/*Project returns the current projected map.*/
func (inv *Inverse) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return inv.next.Project()
}

/*Error reports any execution error.*/
func (inv *Inverse) Error() string {
	if inv.err != nil {
		return inv.err.Error()
	}

	return inv.next.Error()
}

/*Close releases internal state.*/
func (inv *Inverse) Close() error {
	if err := inv.initial.Close(); err != nil {
		return err
	}

	if err := inv.next.Close(); err != nil {
		return err
	}

	inv.err = nil

	return nil
}