package calculus

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Attack walks a value from silence up to a peak. It is Decay inverted: same
clock and shape, target is the peak instead of zero.
*/
type Attack struct {
	clock types.IO[float64]
	shape types.IO[float64]
	peak  types.Input[float64]
	value types.Value[float64]
	err   error
}

var _ types.IO[float64] = (*Attack)(nil)

/*
NewAttack takes a clock and an optional shape. A nil shape is linear.
*/
func NewAttack(clock types.IO[float64], shape types.IO[float64]) *Attack {
	return &Attack{
		clock: clock,
		shape: shape,
		peak:  types.NewInput[float64](),
	}
}

/*
Write stages the peak from the source.
*/
func (attack *Attack) Write(input types.Input[float64]) {
	attack.peak.Write(input)
	attack.err = nil
}

/*
Read executes the risen level and returns the attack as output.
*/
func (attack *Attack) Read() types.Output[float64] {
	if attack.clock == nil {
		attack.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"attack: clock required",
			nil,
		))

		return attack
	}

	peak := attack.peak.Read()

	if peak.Error() != "" {
		attack.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			peak.Error(),
			nil,
		))

		return attack
	}

	progress := attack.clock.Read()

	if progress.Error() != "" {
		attack.err = errnie.Error(errnie.Err(
			errnie.Validation,
			progress.Error(),
			nil,
		))

		return attack
	}

	risen := progress.Project().Read()

	if risen > 1 {
		risen = 1
	}

	if attack.shape != nil {
		attack.shape.Write(progress)
		shaped := attack.shape.Read()

		if shaped.Error() != "" {
			attack.err = errnie.Error(errnie.Err(
				errnie.Validation,
				shaped.Error(),
				nil,
			))

			return attack
		}

		risen = 1 - shaped.Project().Read()
	}

	if risen < 0 {
		risen = 0
	}

	attack.value = attack.value.Write(peak.Project().Read() * risen)
	attack.err = nil

	return attack
}

/*
Project is the last risen level.
*/
func (attack *Attack) Project() types.Value[float64] {
	return attack.value
}

/*
Error reports a staging or execution failure.
*/
func (attack *Attack) Error() string {
	if attack.err == nil {
		return ""
	}

	return attack.err.Error()
}

/*
Close resets and closes owned modifiers.
*/
func (attack *Attack) Close() error {
	if attack.clock != nil {
		if err := attack.clock.Close(); err != nil {
			return err
		}
	}

	if attack.shape != nil {
		if err := attack.shape.Close(); err != nil {
			return err
		}
	}

	if err := attack.peak.Close(); err != nil {
		return err
	}

	attack.value = types.Value[float64]{}
	attack.err = nil

	return nil
}
