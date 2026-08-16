package calculus

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

type scalarMap = types.Map[string, types.Value[float64]]

func scalar(mapping scalarMap, key string) (float64, bool) {
	value, found := mapping.Get(key)

	if !found {
		return 0, false
	}

	return value.Read(), true
}

func putScalar(mapping scalarMap, key string, value float64) {
	mapping.Put(key, types.NewValue(value))
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

func scalarValidation(name string, message string) error {
	return errnie.Error(errnie.Err(
		errnie.Validation,
		name+": "+message,
		nil,
	))
}

func stageScalar(input types.IO[scalarMap], name string) (scalarMap, error) {
	if input == nil {
		return types.NewMap[string, types.Value[float64]](),
			scalarValidation(name, "input is nil")
	}

	if input.Error() != "" {
		return types.NewMap[string, types.Value[float64]](),
			errnie.Error(errnie.Err(errnie.NotFound, input.Error(), nil))
	}

	return input.Project().Read(), nil
}

func scalarInput(mapping scalarMap) types.Input[scalarMap] {
	return types.NewInput(types.NewValue(mapping))
}
