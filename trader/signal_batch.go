package trader

import (
	"sync"

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

	if len(signals) == 1 {
		results[0].measurements, results[0].err = measure(signals[0])
		return results
	}

	waitGroup := sync.WaitGroup{}
	waitGroup.Add(len(signals))

	for index, signal := range signals {
		go func(index int, signal types.Signal[any]) {
			defer waitGroup.Done()
			results[index].measurements, results[index].err = measure(signal)
		}(index, signal)
	}

	waitGroup.Wait()

	return results
}
