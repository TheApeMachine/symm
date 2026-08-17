package learning

import (
	"errors"
)

var (
	ErrEmptyInputs   = errors.New("learning: empty inputs")
	ErrZeroPredicted = errors.New("learning: zero predicted value")
)

func parsePredictedActual(
	primary float64, extras []float64,
) (float64, float64, error) {
	if len(extras) >= 2 {
		predicted := extras[0]
		actual := extras[1]

		if predicted == 0 {
			return 0, 0, ErrZeroPredicted
		}

		return predicted, actual, nil
	}

	if len(extras) == 0 {
		return 0, 0, ErrEmptyInputs
	}

	predicted := primary
	actual := extras[0]

	if predicted == 0 {
		return 0, 0, ErrZeroPredicted
	}

	return predicted, actual, nil
}
