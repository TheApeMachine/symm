package engine

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
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
MeasurementQueue stores signal measurements with bounded backpressure.
*/
type MeasurementQueue struct {
	queue    sync.Map
	seq      atomic.Int64
	pending  atomic.Int64
	dropped  atomic.Int64
	enqueued atomic.Int64
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
	limit := maxPendingPerSignal()

	if measurementQueue.pending.Load() >= limit {
		measurementQueue.dropped.Add(1)

		return fmt.Errorf(
			"signal measurement queue full: pending=%d max=%d",
			measurementQueue.pending.Load(),
			limit,
		)
	}

	globalLimit := maxPendingGlobal()

	if globalLimit > 0 && globalPending.Load() >= globalLimit {
		measurementQueue.dropped.Add(1)

		return fmt.Errorf(
			"global measurement queue full: pending=%d max=%d",
			globalPending.Load(),
			globalLimit,
		)
	}

	measurementQueue.queue.Store(measurementQueue.seq.Add(1), measurement)
	measurementQueue.pending.Add(1)
	measurementQueue.enqueued.Add(1)
	globalPending.Add(1)

	return nil
}

/*
Stats returns live queue counters.
*/
func (measurementQueue *MeasurementQueue) Stats() QueueStats {
	return QueueStats{
		Pending:  measurementQueue.pending.Load(),
		Dropped:  measurementQueue.dropped.Load(),
		Enqueued: measurementQueue.enqueued.Load(),
	}
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
				globalPending.Add(-1)

				return true
			}

			if !yield(measurement) {
				return false
			}

			measurementQueue.queue.Delete(key)
			measurementQueue.pending.Add(-1)
			globalPending.Add(-1)

			return true
		})
	}
}
