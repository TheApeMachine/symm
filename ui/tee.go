package ui

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
UITee is a concrete off-ramp that accepts *data.Measurement[float64]
and yields encoded FlatBuffer []byte frames for the dashboard, as well as
streaming fluid manifold frames over WebRTC.
It satisfies runtime.Tee[*data.Measurement[float64], []byte].
*/
type UITee struct {
	*runtime.System
	queue     *lf.Queue[*data.Measurement[float64]]
	batchSize int
	filters   []func(
		measurement *data.Measurement[float64],
	) bool
}

/*
NewUITee creates a new wait-free UITee off-ramp.
*/
func NewUITee(
	ctx context.Context,
	label string,
	batchSize int,
	filters ...func(
		measurement *data.Measurement[float64],
	) bool,
) *UITee {
	if batchSize < 1 {
		batchSize = 1
	}

	if len(filters) == 0 {
		filters = []func(*data.Measurement[float64]) bool{types.Filters}
	}

	tee := &UITee{
		queue:     lf.NewQueue[*data.Measurement[float64]](),
		batchSize: batchSize,
		filters:   filters,
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	return tee
}

/*
Push receives measurements from the workspace. Raw venue feeds stay off the
dashboard websocket; the page route still selects which analytical sources
are on the wire.
*/
func (tee *UITee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
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

	tee.queue.Enqueue(measurement.Clone())
}

/*
Next drains available measurements from the queue, batches them,
and returns an encoded FlatBuffer frame ([]byte).
*/
func (tee *UITee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn("[tee] pulling from a non-ready system may have unintended consequences")
		return nil
	}

	batch := make([]*data.Measurement[float64], 0, tee.batchSize)

	for len(batch) < tee.batchSize {
		measurement, ok := tee.queue.Dequeue()

		if !ok {
			break
		}

		batch = append(batch, measurement)
	}

	if len(batch) == 0 {
		return nil
	}

	measurements, err := types.EncodeMeasurements(batch)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[tee] Failed to encode measurements",
			err,
		))

		return nil
	}

	return unsafe.Pointer(&measurements)
}

type ManifoldTee struct {
	*runtime.System
	queue     *lf.Queue[*data.Measurement[float64]]
	filters   []func(measurement *data.Measurement[float64]) bool
}

func NewManifoldTee(
	ctx context.Context,
	label string,
	filters ...func(measurement *data.Measurement[float64]) bool,
) *ManifoldTee {
	if len(filters) == 0 {
		filters = []func(*data.Measurement[float64]) bool{types.Filters}
	}

	tee := &ManifoldTee{
		queue:     lf.NewQueue[*data.Measurement[float64]](),
		filters:   filters,
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	return tee
}

func (tee *ManifoldTee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
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

	tee.queue.Enqueue(measurement.Clone())
}

func (tee *ManifoldTee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		return nil
	}

	// We only process one measurement at a time, since each one yields one manifold frame
	measurement, ok := tee.queue.Dequeue()
	if !ok || measurement == nil {
		return nil
	}

	if measurement.Result == nil {
		return nil
	}

	if manifoldState, ok := measurement.Result.(*types.ManifoldState); ok {
		payload, err := types.EncodeManifold(manifoldState)
		if err == nil && payload != nil {
			return unsafe.Pointer(&payload)
		}
	}

	return nil
}
