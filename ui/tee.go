package ui

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
UITee is a concrete off-ramp that accepts *data.Measurement
and yields encoded FlatBuffer []byte frames for the dashboard websocket,
including ManifoldFrame payloads when a measurement carries a ManifoldState.
It satisfies runtime.Tee[*data.Measurement, []byte].
Encoding is executed in a dedicated background worker to eliminate serialization
jitter from the consumer fast path and immediately release retained publications.
*/
type UITee struct {
	*nmruntime.System
	egress  *lf.Queue[*data.Measurement]
	filters []func(
		measurement *data.Measurement,
	) bool
	route  string
	symbol string
}

/*
NewUITee creates a new wait-free UITee off-ramp
with background FlatBuffer encoding.
*/
func NewUITee(
	ctx context.Context,
	label string,
	filters ...func(
		measurement *data.Measurement,
	) bool,
) *UITee {
	if len(filters) == 0 {
		filters = []func(*data.Measurement) bool{types.Filters}
	}

	tee := &UITee{
		filters: filters,
		egress:  lf.NewQueue[*data.Measurement](),
		route:   types.Route(),
		symbol:  types.Focus(),
	}

	tee.System = nmruntime.NewSystem(ctx, label, tee)
	return tee
}

/*
Push receives measurements from the workspace. Raw venue feeds stay off the
dashboard websocket; the page route still selects which analytical sources
are on the wire.
*/
func (tee *UITee) Push(measurement *data.Measurement) {
	if tee.Status() != nmruntime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	if measurement == nil {
		return
	}

	for _, filter := range tee.filters {
		if !filter(measurement) {
			return
		}
	}

	tee.egress.Enqueue(measurement)
}

/*
Next yields the next available pre-encoded FlatBuffer frame ([]byte).
Encoding is owned exclusively by the background worker, so Next is an
instantaneous wait-free dequeue that preserves the worker's encode order.
*/
func (tee *UITee) Next() unsafe.Pointer {
	if tee.Status() != nmruntime.READY {
		errnie.Warn("[tee] pulling from a non-ready system may have unintended consequences")
		return nil
	}

	payload, ok := tee.egress.Dequeue()

	if !ok {
		return nil
	}

	encoded, err := data.EncodeMeasurements([]*data.Measurement{payload})

	if err != nil {
		return nil
	}

	return unsafe.Pointer(&encoded)
}

/*
Pending returns the total backlog of un-transmitted items across ingress and egress.
*/
func (tee *UITee) Pending() int {
	return int(tee.egress.Length())
}
