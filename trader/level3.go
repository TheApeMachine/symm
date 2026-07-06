package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Level3 struct {
	signals []types.Signal[any]
}

func NewLevel3(signals []types.Signal[any]) *Level3 {
	return &Level3{
		signals: signals,
	}
}

func (level3 *Level3) Measure(message kraken.Level3DataSlice) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		for _, signal := range level3.signals {
			measurement, err := signal.Measure(msg, &types.CrossSection{})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}

			measurements = append(measurements, measurement...)
		}
	}

	return measurements, nil
}
