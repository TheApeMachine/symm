package ui

import (
	"context"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/spf13/viper"
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
	queue              *lf.Queue[*data.Measurement[float64]]
	bound              uint64
	snapshot           func() *types.ManifoldState
	projectionInterval time.Duration
	lastProjection     atomic.Int64
}

/*
NewUITee creates a new wait-free UITee off-ramp.
*/
func NewUITee(ctx context.Context, label string, capacity int) *UITee {
	if capacity < 1 {
		capacity = 1
	}

	viper.SetDefault("ui.websocket.learning_interval", "250ms")
	tee := &UITee{
		queue:              lf.NewQueue[*data.Measurement[float64]](),
		bound:              uint64(capacity),
		projectionInterval: viper.GetDuration("ui.websocket.learning_interval"),
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

	if !types.AllowsRoute(measurement) {
		return
	}

	if tee.queue.Length() >= tee.bound {
		return
	}

	if _, projection := measurement.Result.(interface{ Snapshot() any }); projection {
		now := time.Now().UnixNano()
		previous := tee.lastProjection.Load()

		if now-previous < int64(tee.projectionInterval) || !tee.lastProjection.CompareAndSwap(previous, now) {
			return
		}
	}

	publication := measurement.Clone()

	if projection, ok := measurement.Result.(interface{ Snapshot() any }); ok {
		publication.Result = projection.Snapshot()
	}

	tee.queue.Enqueue(publication)
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

	const batchCapacity = 32
	batch := make([]*data.Measurement[float64], 0, batchCapacity)

	for len(batch) < batchCapacity {
		measurement, ok := tee.queue.Dequeue()

		if !ok {
			break
		}

		if types.AllowsRoute(measurement) {
			batch = append(batch, measurement)
		}
	}

	if len(batch) == 0 {
		return nil
	}

	var payload []byte

	err := types.EncodeMeasurementsFrameWith(batch, func(frame []byte) error {
		payload = make([]byte, len(frame))
		copy(payload, frame)
		return nil
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"ui tee: failed to encode measurements frame",
			err,
		))

		return nil
	}

	return unsafe.Pointer(&payload)
}
