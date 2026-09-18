package signal

import (
	"context"
	"iter"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the universal signal entity in symm. It owns its declared Flume
pipeline topology, its ingress connections registered on the grid, its
retained metric holds, and its authoritative shared Measurement.

Next does not read from the grid: it reads locally from its retained metric
holds, writes them to the measurement, and yields the measurement pointer.
*/
type Signal struct {
	*runtime.System

	name        string
	source      string
	symbol      string
	measurement *data.Measurement[float64]
	holds       map[string]*store.Retained[float64]
	conns       []*transport.Conn[*geometry.Coordinate]
	seqIdx      atomic.Int64
}

func NewSignal(
	ctx context.Context,
	name string,
	source string,
	symbol string,
) *Signal {
	measurement := data.NewMeasurement[float64](source, make(map[string]data.Metric[float64]))
	measurement.Label = symbol

	signal := &Signal{
		name:        name,
		source:      source,
		symbol:      symbol,
		measurement: measurement,
		holds:       make(map[string]*store.Retained[float64]),
		conns:       make([]*transport.Conn[*geometry.Coordinate], 0),
	}

	signal.System = runtime.NewSystem(ctx, name, signal)
	signal.Transition(runtime.READY)
	return signal
}

func (signal *Signal) Name() string {
	return signal.name
}

func (signal *Signal) Source() string {
	return signal.source
}

func (signal *Signal) Symbol() string {
	return signal.symbol
}

func (signal *Signal) Holds() map[string]*store.Retained[float64] {
	return signal.holds
}

func (signal *Signal) Measurement() *data.Measurement[float64] {
	return signal.measurement
}

/*
Next reads locally from retained metric holds, updates the signal's
authoritative measurement, and yields its pointer.
*/
func (signal *Signal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in != nil {
			for range in {
			}
		}

		if signal.Status() != runtime.READY {
			errnie.Warn(signal.Name() + ": Next called before READY; dropping event")
			return
		}

		now := time.Now()
		signal.measurement.At = now
		signal.measurement.Timestamp = now.UnixNano()
		signal.measurement.SeqIdx = signal.seqIdx.Add(1)

		for metricName, hold := range signal.holds {
			for valPtr := range hold.Next(nil) {
				if valPtr == nil {
					continue
				}

				val := *(*float64)(valPtr)
				metric, found := signal.measurement.Metrics[metricName]

				if !found {
					metric = data.NewMetric[float64](metricName, "", "", 0, 0)
				}

				signal.measurement.Metrics[metricName] = metric.Write(val)
			}
		}

		yield(unsafe.Pointer(&signal.measurement))
	}
}

// Ensure Signal satisfies core.Primitive.
var _ core.Primitive = (*Signal)(nil)
