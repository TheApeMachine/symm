package calculus

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Decay walks a numeric value toward zero. Without clock or shape, it zeroes
out the value in one iteration. When clock or shape are provided in the map,
it modulates the decayed level.
*/
type Decay struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Decay)(nil)

/*
NewDecay instantiates a Decay primitive.
*/
func NewDecay(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Decay {
	return &Decay{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*
Write stages the decay map.
*/
func (decay *Decay) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		decay.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"decay: input is nil",
			nil,
		))

		return
	}

	decay.next.Write(input)
	decay.err = nil
}

/*
Read computes the decayed level.
*/
func (decay *Decay) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := decay.next.Read()

	if in.Error() != "" {
		decay.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return decay.next
	}

	mapping := in.Project().Read()
	levelVal, hasLevel := mapping.Get("level")

	if !hasLevel {
		decay.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"decay: missing level",
			nil,
		))

		return decay.next
	}

	level := levelVal.Read()
	clockVal, hasClock := mapping.Get("clock")
	shapeVal, hasShape := mapping.Get("shape")

	if !hasClock {
		mapping.Put("level", types.NewValue(0.0))
		mapping.Put("result", types.NewValue(0.0))

		decay.next.Write(types.NewInput(types.NewValue(mapping)))
		decay.err = nil

		return decay.next
	}

	remaining := 1.0 - clockVal.Read()

	if remaining < 0 {
		remaining = 0
	}

	if hasShape {
		remaining = shapeVal.Read()
	}

	result := level * remaining
	mapping.Put("level", types.NewValue(result))
	mapping.Put("result", types.NewValue(result))

	decay.next.Write(types.NewInput(types.NewValue(mapping)))
	decay.err = nil

	return decay.next
}

/*
Project returns the current projected map.
*/
func (decay *Decay) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return decay.next.Project()
}

/*
Error reports any execution error.
*/
func (decay *Decay) Error() string {
	if decay.err != nil {
		return decay.err.Error()
	}

	return decay.next.Error()
}

/*
Close releases internal state.
*/
func (decay *Decay) Close() error {
	if err := decay.initial.Close(); err != nil {
		return err
	}

	if err := decay.next.Close(); err != nil {
		return err
	}

	decay.err = nil

	return nil
}
