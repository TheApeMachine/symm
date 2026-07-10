package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type signalMeasurement struct {
	measurements []*types.Measurement
	err          error
}

func measureSignals(
	signals []types.Signal[any],
	measure func(types.Signal[any]) ([]*types.Measurement, error),
) []signalMeasurement {
	results := make([]signalMeasurement, len(signals))

	for index, signal := range signals {
		results[index].measurements, results[index].err = measure(signal)

		if results[index].err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				results[index].err.Error(),
				results[index].err,
			))
		}
	}

	return results
}
