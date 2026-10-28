package adaptive

import (
	"math"

	"github.com/theapemachine/errnie"
)

func finiteAdaptive(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			name+": value must be finite",
			nil,
		))
	}

	return nil
}
