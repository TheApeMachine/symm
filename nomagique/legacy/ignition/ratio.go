package equation

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*Ratio normalizes evidence only when its empirical baseline is ready.
Its map carries "value", "baseline", and "ready", producing "result".*/
type Ratio struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Ratio)(nil)

/*NewRatio returns a Ratio normalization primitive.*/
func NewRatio(initial types.Input[types.Map[string, types.Value[float64]]]) *Ratio {
	return &Ratio{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*Write stages the ratio map.*/
func (ratio *Ratio) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		ratio.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"ratio: input is nil",
			nil,
		))

		return
	}

	ratio.next.Write(input)
	ratio.err = nil
}

/*Read computes the ratio result = value / baseline if ready, else 0.*/
func (ratio *Ratio) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := ratio.next.Read()

	if in.Error() != "" {
		ratio.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return ratio.next
	}

	mapping := in.Project().Read()
	valueVal, hasValue := mapping.Get("value")
	baselineVal, hasBaseline := mapping.Get("baseline")
	readyVal, hasReady := mapping.Get("ready")

	if !hasValue || !hasBaseline || !hasReady {
		ratio.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"ratio: missing value, baseline, or ready",
			nil,
		))

		return ratio.next
	}

	value := valueVal.Read()
	baseline := baselineVal.Read()
	ready := readyVal.Read()

	if !ready || baseline <= 0 || value <= 0 {
		mapping.Put("result", types.NewValue(0.0))
		ratio.next.Write(types.NewInput(types.NewValue(mapping)))
		ratio.err = nil

		return ratio.next
	}

	result := value / baseline
	mapping.Put("result", types.NewValue(result))

	ratio.next.Write(types.NewInput(types.NewValue(mapping)))
	ratio.err = nil

	return ratio.next
}

/*Project returns the current projected map.*/
func (ratio *Ratio) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return ratio.next.Project()
}

/*Error reports any execution error.*/
func (ratio *Ratio) Error() string {
	if ratio.err != nil {
		return ratio.err.Error()
	}

	return ratio.next.Error()
}

/*Close releases internal state.*/
func (ratio *Ratio) Close() error {
	if err := ratio.initial.Close(); err != nil {
		return err
	}

	if err := ratio.next.Close(); err != nil {
		return err
	}

	ratio.err = nil

	return nil
}