package calculus

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Attack applies an excitation impulse or jump to a baseline or existing value.
Its map carries "base", "jump", and "result".
*/
type Attack struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Attack)(nil)

/*
NewAttack returns an Attack excitation primitive.
*/
func NewAttack(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Attack {
	return &Attack{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*
Write stages the jump map.
*/
func (attack *Attack) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		attack.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"attack: input is nil",
			nil,
		))

		return
	}

	attack.next.Write(input)
	attack.err = nil
}

/*
Read adds the impulse to the baseline and returns the new level.
*/
func (attack *Attack) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := attack.next.Read()

	if in.Error() != "" {
		attack.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return attack.next
	}

	mapping := in.Project().Read()
	baseVal, _ := mapping.Get("base")
	jumpVal, hasJump := mapping.Get("jump")

	if !hasJump {
		attack.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"attack: missing jump",
			nil,
		))

		return attack.next
	}

	result := baseVal.Read() + jumpVal.Read()
	mapping.Put("base", types.NewValue(result))
	mapping.Put("result", types.NewValue(result))

	attack.next.Write(types.NewInput(types.NewValue(mapping)))
	attack.err = nil

	return attack.next
}

/*
Project returns the current projected map.
*/
func (attack *Attack) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return attack.next.Project()
}

/*
Error reports any execution error.
*/
func (attack *Attack) Error() string {
	if attack.err != nil {
		return attack.err.Error()
	}

	return attack.next.Error()
}

/*
Close resets the primitive state.
*/
func (attack *Attack) Close() error {
	if err := attack.initial.Close(); err != nil {
		return err
	}

	if err := attack.next.Close(); err != nil {
		return err
	}

	attack.err = nil

	return nil
}
