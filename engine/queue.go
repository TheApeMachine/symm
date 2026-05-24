package engine

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/symm/config"
)

var globalPending atomic.Int64

/*
QueueStats exposes measurement queue counters for telemetry.
*/
type QueueStats struct {
	Pending  int64
	Dropped  int64
	Enqueued int64
}

/*
MeasurementQueue stores signal measurements with bounded FIFO backpressure.
*/
type MeasurementQueue struct {
	mu       sync.Mutex
	queue    []Measurement
	dropped  int64
	enqueued int64
}

func maxPendingPerSignal() int64 {
	if config.System.MaxPendingPerSignal > 0 {
		return int64(config.System.MaxPendingPerSignal)
	}

	return 4096
}

func maxPendingGlobal() int64 {
	return int64(config.System.MaxPendingGlobal)
}

/*
Enqueue stores one measurement when capacity allows.
*/
func (measurementQueue *MeasurementQueue) Enqueue(measurement Measurement) error {
	measurementQueue.mu.Lock()
	defer measurementQueue.mu.Unlock()

	limit := maxPendingPerSignal()

	if int64(len(measurementQueue.queue)) >= limit {
		measurementQueue.dropped++

		return fmt.Errorf(
			"signal measurement queue full: pending=%d max=%d",
			len(measurementQueue.queue),
			limit,
		)
	}

	globalLimit := maxPendingGlobal()

	if globalLimit > 0 && globalPending.Load() >= globalLimit {
		measurementQueue.dropped++

		return fmt.Errorf(
			"global measurement queue full: pending=%d max=%d",
			globalPending.Load(),
			globalLimit,
		)
	}

	measurementQueue.queue = append(measurementQueue.queue, measurement)
	measurementQueue.enqueued++
	globalPending.Add(1)

	return nil
}

/*
Stats returns live queue counters.
*/
func (measurementQueue *MeasurementQueue) Stats() QueueStats {
	measurementQueue.mu.Lock()
	defer measurementQueue.mu.Unlock()

	return QueueStats{
		Pending:  int64(len(measurementQueue.queue)),
		Dropped:  measurementQueue.dropped,
		Enqueued: measurementQueue.enqueued,
	}
}

/*
Drain yields queued measurements in FIFO order for the trader.
*/
func (measurementQueue *MeasurementQueue) Drain(_ context.Context) iter.Seq[Measurement] {
	return func(yield func(Measurement) bool) {
		for {
			measurementQueue.mu.Lock()

			if len(measurementQueue.queue) == 0 {
				measurementQueue.mu.Unlock()

				return
			}

			measurement := measurementQueue.queue[0]
			copy(measurementQueue.queue, measurementQueue.queue[1:])
			measurementQueue.queue = measurementQueue.queue[:len(measurementQueue.queue)-1]
			measurementQueue.mu.Unlock()

			globalPending.Add(-1)

			if !yield(measurement) {
				return
			}
		}
	}
}
