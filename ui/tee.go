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
Encoding is executed in a dedicated background worker to eliminate serialization
jitter from the consumer fast path and immediately release retained publications.
*/
type UITee struct {
	*runtime.System
	ingress             *lf.Queue[data.Publication]
	egress              *lf.Queue[[]byte]
	wake                chan struct{}
	available           chan struct{}
	lastManifoldVersion uint64
	filters             []func(
		measurement *data.Measurement[float64],
	) bool
}

/*
NewUITee creates a new wait-free UITee off-ramp with background FlatBuffer encoding.
*/
func NewUITee(
	ctx context.Context,
	label string,
	filters ...func(
		measurement *data.Measurement[float64],
	) bool,
) *UITee {
	if len(filters) == 0 {
		filters = []func(*data.Measurement[float64]) bool{types.Filters}
	}

	tee := &UITee{
		ingress:   lf.NewQueue[data.Publication](),
		egress:    lf.NewQueue[[]byte](),
		wake:      make(chan struct{}, 1),
		available: make(chan struct{}, 1),
		filters:   filters,
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	go tee.worker(tee.Context())
	return tee
}

/*
Available returns a receive-only notification channel signaled whenever
a new encoded frame is ready in the egress queue.
*/
func (tee *UITee) Available() <-chan struct{} {
	return tee.available
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

	select {
	case tee.wake <- struct{}{}:
	default:
	}
}

/*
Next yields the next available pre-encoded FlatBuffer frame ([]byte).
Encoding is performed asynchronously by the background worker, ensuring Next
is an instantaneous wait-free dequeue.
*/
func (tee *UITee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn("[tee] pulling from a non-ready system may have unintended consequences")
		return nil
	}

	payload, ok := tee.egress.Dequeue()

	if ok {
		if len(payload) == 0 {
			return nil
		}

		return unsafe.Pointer(&payload)
	}

	pub, ok := tee.ingress.Dequeue()

	if !ok {
		return nil
	}

	payload = tee.encode(pub)

	if len(payload) == 0 {
		return nil
	}

	return unsafe.Pointer(&payload)
}

func (tee *UITee) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			tee.drainIngress()
			return
		case <-tee.wake:
			for {
				pub, ok := tee.ingress.Dequeue()

				if !ok {
					break
				}

				payload := tee.encode(pub)

				if len(payload) == 0 {
					continue
				}

				if tee.egress.Length() >= 4096 {
					tee.egress.Dequeue()
				}

				tee.egress.Enqueue(payload)

				select {
				case tee.available <- struct{}{}:
				default:
				}
			}
		}
	}
}

func (tee *UITee) encode(pub data.Publication) []byte {
	measurement := pub.Measurement

	if manifoldState, ok := measurement.Result.(*types.ManifoldState); ok {
		if manifoldState == nil || (manifoldState.Version != 0 && manifoldState.Version == tee.lastManifoldVersion) {
			pub.Release()
			return nil
		}

		tee.lastManifoldVersion = manifoldState.Version
		payload, err := types.EncodeManifold(manifoldState)
		pub.Release()

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[tee] Failed to encode manifold",
				err,
			))
			return nil
		}

		return payload
	}

	batch := [1]*data.Measurement[float64]{measurement}
	payload, err := types.EncodeMeasurements(batch[:])
	pub.Release()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[tee] Failed to encode measurements",
			err,
		))
		return nil
	}

	return payload
}

func (tee *UITee) drainIngress() {
	for {
		pub, ok := tee.ingress.Dequeue()

		if !ok {
			return
		}

		pub.Release()
	}
}
