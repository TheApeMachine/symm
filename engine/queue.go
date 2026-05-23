package engine

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
)

const maxPendingMeasurements = 4096

/*
MeasurementQueue stores signal measurements with bounded backpressure.
*/
type MeasurementQueue struct {
	queue   sync.Map
	seq     atomic.Int64
	pending atomic.Int64
}

/*
Enqueue stores one measurement when capacity allows.
*/
func (measurementQueue *MeasurementQueue) Enqueue(measurement Measurement) {
	if measurementQueue.pending.Load() >= maxPendingMeasurements {
		return
	}

	measurementQueue.queue.Store(measurementQueue.seq.Add(1), measurement)
	measurementQueue.pending.Add(1)
}

/*
Drain yields queued measurements for the trader.
*/
func (measurementQueue *MeasurementQueue) Drain(_ context.Context) iter.Seq[Measurement] {
	return func(yield func(Measurement) bool) {
		measurementQueue.queue.Range(func(key, value any) bool {
			measurement, ok := value.(Measurement)

			if !ok {
				errnie.Error(fmt.Errorf("invalid measurement type: %T", value))
				measurementQueue.queue.Delete(key)
				measurementQueue.pending.Add(-1)

				return true
			}

			if !yield(measurement) {
				return false
			}

			measurementQueue.queue.Delete(key)
			measurementQueue.pending.Add(-1)

			return true
		})
	}
}
