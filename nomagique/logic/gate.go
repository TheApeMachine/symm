package logic

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

type scalarMap = types.Map[string, types.Value[float64]]

/*
Gate passes "value" to "result" when numeric "condition" is non-zero and
otherwise writes zero.
*/
type Gate struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*Gate)(nil)

func NewGate(initial types.Input[scalarMap]) *Gate {
	return &Gate{initial: initial, next: types.NewInput[scalarMap]()}
}

func (gate *Gate) Write(input types.IO[scalarMap]) {
	if input == nil {
		mapping := types.NewMap[string, types.Value[float64]]()
		gate.next = types.NewErrorInput(mapping, gateError("input is nil"))
		return
	}
	if input.Error() != "" {
		mapping := types.NewMap[string, types.Value[float64]]()
		gate.next = types.NewErrorInput(mapping,
			errnie.Error(errnie.Err(errnie.NotFound, input.Error(), nil)))
		return
	}
	gate.next = types.NewInput(types.NewValue(input.Project().Read()))
}

func (gate *Gate) Read() types.IO[scalarMap] {
	if gate.next.Error() != "" {
		return gate.next
	}

	mapping := gate.next.Project().Read()
	conditionValue, hasCondition := mapping.Get("condition")
	valueValue, hasValue := mapping.Get("value")
	if !hasCondition || !hasValue {
		gate.next = types.NewErrorInput(mapping,
			gateError("missing condition or value"))
		return gate.next
	}

	condition := conditionValue.Read()
	value := valueValue.Read()
	if math.IsNaN(condition) || math.IsInf(condition, 0) ||
		math.IsNaN(value) || math.IsInf(value, 0) {
		gate.next = types.NewErrorInput(mapping,
			gateError("condition and value must be finite"))
		return gate.next
	}

	result := 0.0
	if condition != 0 {
		result = value
	}
	mapping.Put("result", types.NewValue(result))
	gate.initial = types.NewInput(types.NewValue(mapping))
	gate.next = types.NewInput(types.NewValue(mapping))
	return gate.next
}

func (gate *Gate) Project() types.Value[scalarMap] { return gate.next.Project() }
func (gate *Gate) Error() string                   { return gate.next.Error() }
func (gate *Gate) Close() error {
	if gate.initial != nil {
		if err := gate.initial.Close(); err != nil {
			return err
		}
	}
	if gate.next != nil {
		if err := gate.next.Close(); err != nil {
			return err
		}
	}
	gate.next = types.NewInput[scalarMap]()
	return nil
}

func gateError(message string) error {
	return errnie.Error(errnie.Err(errnie.Validation, "gate: "+message, nil))
}
