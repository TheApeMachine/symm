package ui

import (
	"context"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
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
	*runtime.System
	ingress *lf.Queue[data.Publication]
	egress  *lf.Queue[[]byte]
	filters []func(
		measurement *data.Measurement,
	) bool
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
		ingress: lf.NewQueue[data.Publication](),
		egress:  lf.NewQueue[[]byte](),
		filters: filters,
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	go tee.worker(tee.Context())
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
	tee.ingress.Enqueue(pub)
}

/*
Next yields the next available pre-encoded FlatBuffer frame ([]byte).
Encoding is owned exclusively by the background worker, so Next is an
instantaneous wait-free dequeue that preserves the worker's encode order.
*/
func (tee *UITee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn("[tee] pulling from a non-ready system may have unintended consequences")
		return nil
	}

	payload, ok := tee.egress.Dequeue()

	if !ok || len(payload) == 0 {
		return nil
	}

	return unsafe.Pointer(&payload)
}

func (tee *UITee) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for {
				pub, ok := tee.ingress.Dequeue()

				if !ok {
					break
				}

				pub.Release()
			}

			return
		default:
			if tee.ingress.Length() == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		for tee.ingress.Length() > 0 {
			pub, ok := tee.ingress.Dequeue()

			if !ok {
				break
			}

			payload, err := types.EncodeMeasurements([]*data.Measurement{pub.Measurement})
			pub.Release()

			if err != nil {
				tee.Error(errnie.Err(
					errnie.UnprocessableContent,
					"[tee] Failed to encode measurement",
					err,
				))
				continue
			}

			tee.egress.Enqueue(payload)
		}
	}
}
