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
and yields encoded FlatBuffer []byte frames for the dashboard websocket,
including ManifoldFrame payloads when a measurement carries a ManifoldState.
It satisfies runtime.Tee[*data.Measurement[float64], []byte].
*/
const uiTeeQueueCap = 4096

type UITee struct {
	*runtime.System
	queue     *lf.Queue[data.Publication]
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
		queue:     lf.NewQueue[data.Publication](),
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
func (tee *UITee) Push(pub data.Publication) {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	if pub.Measurement == nil {
		return
	}

	for _, filter := range tee.filters {
		if !filter(pub.Measurement) {
			return
		}
	}

	pub.Retain()

	if tee.queue.Length() >= uiTeeQueueCap {
		pub.Release()
		return
	}

	tee.queue.Enqueue(pub)
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
	batchPubs := make([]data.Publication, 0, tee.batchSize)

	for len(batch) < tee.batchSize {
		pub, ok := tee.queue.Dequeue()

		if !ok {
			break
		}

		measurement := pub.Measurement
		dropped := false

		for _, filter := range tee.filters {
			if !filter(measurement) {
				dropped = true
				break
			}
		}

		if dropped {
			pub.Release()
			continue
		}

		if manifoldState, ok := measurement.Result.(*types.ManifoldState); ok {
			if len(batch) > 0 {
				tee.queue.Enqueue(pub)
				break
			}

			payload, err := types.EncodeManifold(manifoldState)
			pub.Release()

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"[tee] Failed to encode manifold",
					err,
				))
				continue
			}

			if payload == nil {
				continue
			}

			return unsafe.Pointer(&payload)
		}

		batch = append(batch, measurement)
		batchPubs = append(batchPubs, pub)
	}

	if len(batch) == 0 {
		return nil
	}

	measurements, err := types.EncodeMeasurements(batch)
	for _, pub := range batchPubs {
		pub.Release()
	}

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[tee] Failed to encode measurements",
			err,
		))

		return nil
	}

	if measurements == nil {
		return nil
	}

	return unsafe.Pointer(&measurements)
}
